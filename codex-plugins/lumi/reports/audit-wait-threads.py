#!/usr/bin/env python3
"""Audit frozen real Codex rollouts referenced by storyboard_audit.py output.

Reads host records only; does not inspect Lumi project internals or infer tokens.
Model calls mean distinct successful cumulative token-usage updates, matching
the original acceptance's 61-call measure; unreported failed attempts are unknown.
"""
import argparse
from collections import Counter
from datetime import datetime
import hashlib
import json
from pathlib import Path
import re


KEYS = ('input_tokens', 'cached_input_tokens', 'output_tokens')
WAIT = re.compile(r'tools\.mcp__codex_app__wait_threads\s*\(')
CELL = re.compile(r'Script running with cell ID ([^\s]+)')


def text_values(value):
    if isinstance(value, str):
        return value
    if isinstance(value, list):
        return '\n'.join(text_values(item) for item in value)
    if isinstance(value, dict):
        return '\n'.join(text_values(item) for item in value.values())
    return ''


def wait_events(events):
    """Inspect resolved MCP arguments/results, rather than guessing store/load values."""
    rows, cursors, seen = [], {}, set()
    for line, event in enumerate(events, 1):
        payload = event.get('payload', {})
        item = payload.get('item', {})
        if payload.get('type') != 'item_completed' or item.get('tool') != 'wait_threads':
            continue
        if item['id'] in seen:
            continue
        seen.add(item['id'])
        result = item.get('result') or {}
        data = result.get('structuredContent')
        if data is None:
            texts = [c['text'] for c in result.get('content', []) if c.get('type') == 'text']
            try:
                data = json.loads(texts[0]) if texts else {}
            except ValueError:
                data = {}
        duration = item.get('duration', {})
        seconds = duration.get('secs', 0) + duration.get('nanos', 0) / 1e9
        targets = []
        for target in item.get('arguments', {}).get('targets', []):
            poll = next((p for p in data.get('polls', []) if p['thread']['id'] == target['threadId']
                         and (not target.get('hostId') or p['thread']['hostId'] == target['hostId'])), {})
            key = (target['threadId'], poll.get('thread', {}).get('hostId', target.get('hostId')))
            prior = cursors.get(key)
            targets.append({**target, 'previous_returned_cursor': prior,
                            'cursor_matches_previous': target.get('afterCursor') == prior if prior else None,
                            'returned_cursor': poll.get('cursor'), 'status': poll.get('thread', {}).get('status'),
                            'latest_turn_status': poll.get('latestTurn', {}).get('status')})
            if poll.get('cursor') is not None:
                cursors[key] = poll['cursor']
        rows.append({'line': line, 'item_id': item['id'], 'timestamp': event['timestamp'],
                     'duration_seconds': seconds, 'timeout_ms': item.get('arguments', {}).get('timeoutMs'),
                     'timed_out': data.get('timedOut'), 'wake': data.get('wake'),
                     'errors': data.get('errors'), 'is_error': result.get('isError', False), 'targets': targets})
    return rows


def audit_session(session):
    path = Path(session['rollout'])
    raw = path.read_bytes()
    digest = hashlib.sha256(raw).hexdigest()
    if digest != session['rollout_sha256']:
        raise ValueError(f'changed frozen rollout: {path}')
    events = [json.loads(line) for line in raw.splitlines() if line.strip()]
    calls, cells, extra, other = {}, {}, [], []
    usage_updates, last = set(), None
    tool_counts = Counter()
    chapter_waits = []
    for line, event in enumerate(events, 1):
        payload = event.get('payload', {})
        if event['type'] == 'event_msg' and payload.get('type') == 'token_count' and payload.get('info'):
            current = payload['info']['total_token_usage']
            value = tuple(current[key] for key in KEYS)
            if last and any(a < b for a, b in zip(value, last)):
                raise ValueError(f'counter reset in {path}:{line}')
            if not 0 <= value[1] <= value[0]:
                raise ValueError(f'invalid cached usage in {path}:{line}')
            if any(value):
                usage_updates.add(value)
            last = value
        if event['type'] != 'response_item':
            continue
        kind = payload.get('type')
        if kind in ('function_call', 'custom_tool_call'):
            name = payload.get('name', '').split('.')[-1]
            tool_counts[name] += 1
            code = payload.get('input', payload.get('arguments', ''))
            call = {'call_id': payload['call_id'], 'name': name, 'line': line,
                    'timestamp': event['timestamp'], 'input': code}
            calls[payload['call_id']] = call
            if name == 'exec' and WAIT.search(code):
                pragma = re.match(r'// @exec:\s*(\{[^\n]+\})\n', code)
                outer = json.loads(pragma[1]).get('yield_time_ms') if pragma else None
                inner = re.findall(r'timeoutMs\s*:\s*(\d+)', code)
                chapter_waits.append({**call, 'outer_yield_time_ms': outer,
                                      'inner_timeout_ms': [int(v) for v in inner]})
            elif name == 'wait':
                args = json.loads(code)
                source = cells.get(args['cell_id'])
                entry = {**call, 'cell_id': args['cell_id'], 'source_exec': source}
                if source and WAIT.search(source['input']):
                    extra.append(entry)
                else:
                    other.append(entry)
        elif kind in ('custom_tool_call_output', 'function_call_output'):
            # The yield marker is in the actual outer tool output, not the model's code.
            output = text_values(payload.get('output', ''))
            match = CELL.search(output)
            source = calls.get(payload.get('call_id'))
            if match and source:
                # A repeated yield on functions.wait keeps the original exec identity.
                cells[match[1]] = cells.get(match[1], source)
    if last is None:
        raise ValueError(f'no actual usage in {path}')
    recorded = session['usage']
    if last != tuple(recorded[key] for key in KEYS):
        raise ValueError(f'usage audit differs from frozen rollout: {path}')
    meta = events[0]['payload']
    return {key: session.get(key) for key in
            ('thread_id', 'kind', 'parent_thread_id', 'agent_path', 'models', 'usage')} | {
        'rollout': str(path), 'rollout_sha256': digest, 'host_cli_version': meta.get('cli_version'),
        'model_calls': len(usage_updates), 'tool_calls': dict(tool_counts),
        'wait_threads_calls': chapter_waits, 'extra_functions_wait': extra,
        'other_functions_wait': other, 'resolved_wait_events': wait_events(events),
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--usage-audit', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    data = json.loads(args.usage_audit.read_text())
    sessions = [audit_session(session) for session in data['sessions']]
    root = next(s for s in sessions if s['kind'] == 'root')
    elapsed = (datetime.fromisoformat(data['end'].replace('Z', '+00:00'))
               - datetime.fromisoformat(data['start'].replace('Z', '+00:00'))).total_seconds()
    result = {'source_usage_audit': str(args.usage_audit.resolve()),
              'start': data['start'], 'end': data['end'], 'elapsed_seconds': elapsed,
              'root_thread': data['root_thread'], 'root_model_calls': root['model_calls'],
              'root_wait_threads_calls': len(root['wait_threads_calls']),
              'root_extra_functions_wait': len(root['extra_functions_wait']),
              'root_other_functions_wait': len(root['other_functions_wait']),
              'workflow_extra_functions_wait': sum(len(s['extra_functions_wait']) for s in sessions
                                                 if s['kind'] != 'host_auxiliary'),
              'workflow_other_functions_wait': sum(len(s['other_functions_wait']) for s in sessions
                                                 if s['kind'] != 'host_auxiliary'),
              'workflow_totals': data['workflow_totals'], 'including_host_auxiliary': data['totals'],
              'sessions': sessions,
              'notes': ['input_tokens includes cached_input_tokens',
                        'uncached = input - cached; output already includes reasoning',
                        'model_calls counts distinct successful cumulative usage updates',
                        'each extra wait is linked by the actual yielded cell to its outer exec',
                        'elapsed includes real interruptions/resume until root final completion']}
    args.output.write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps({k: v for k, v in result.items() if k not in ('sessions', 'notes')}, ensure_ascii=False))


if __name__ == '__main__':
    main()
