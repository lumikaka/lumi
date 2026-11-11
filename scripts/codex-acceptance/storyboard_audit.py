#!/usr/bin/env python3
"""Read actual Codex rollout counters and model-visible tools for one fixture.

Never estimates tokens or sums repeated cumulative updates. Sessions are selected
by the disposable project's exact cwd and bounded by the root's start/end time.
Independent chapter chats and all descendant subagents are included. Run before
resuming these chats for a different acceptance phase, and retain the JSON result.
"""
import argparse
import hashlib
import json
import re
from pathlib import Path

from storyboard_fixture import NOVEL


def read_jsonl(path):
    with path.open() as f:
        for line in f:
            if line.strip():
                yield json.loads(line)


def metadata(path):
    # Only inspect metadata of unrelated sessions, never their chat content.
    with path.open() as f:
        first = json.loads(f.readline())
    return first['payload'] if first.get('type') == 'session_meta' else {}


def usage(events):
    latest, peak, observations = None, 0, 0
    for event in events:
        p = event.get('payload', {})
        if event.get('type') != 'event_msg' or p.get('type') != 'token_count' or not p.get('info'):
            continue
        info = p['info']
        value = info.get('total_token_usage')
        if value is None:
            continue
        if latest and any(value[k] < latest[k] for k in ('input_tokens', 'cached_input_tokens', 'output_tokens')):
            raise ValueError('usage counter reset: split this session into explicit measurement windows')
        if not 0 <= value['cached_input_tokens'] <= value['input_tokens']:
            raise ValueError('invalid cached input counter')
        latest = value
        last = info.get('last_token_usage')
        if last:
            peak = max(peak, last['input_tokens'])
        observations += 1
    if latest is None or peak == 0:
        raise ValueError('real usage or per-request input is unavailable; do not report zero')
    return {'input_tokens': latest['input_tokens'], 'cached_input_tokens': latest['cached_input_tokens'],
            'uncached_input_tokens': latest['input_tokens'] - latest['cached_input_tokens'],
            'output_tokens': latest['output_tokens'], 'single_input_peak': peak,
            'usage_observations': observations}


def tool_items(events):
    return [event['payload'] for event in events if event.get('type') == 'response_item'
            and event.get('payload', {}).get('type') in ('function_call', 'function_call_output', 'custom_tool_call', 'custom_tool_call_output')]


def text_forms(value):
    """Unwrap JSON tool envelopes and escaped text, without executing anything."""
    if isinstance(value, dict):
        return '\n'.join(text_forms(v) for v in value.values())
    if isinstance(value, list):
        return '\n'.join(text_forms(v) for v in value)
    if not isinstance(value, str):
        return ''
    try:
        decoded = json.loads(value)
    except (ValueError, TypeError):
        return value.replace('\\n', '\n').replace('\\"', '"')
    return text_forms(decoded) if decoded != value else value


def content_audit(items, novel, pages):
    visible = ''.join(text_forms(items).split())
    sources = []
    for code, _, text in novel:
        paragraphs = [''.join(p.split()) for p in text.strip().splitlines() if p.strip()]
        found = [p in visible for p in paragraphs]
        sources.append({'chapter_code': code, 'paragraphs': len(found), 'paragraphs_found': sum(found),
                        'full_source_found': all(found)})
    whole_draft = []
    for chapter in pages:
        values = [''.join((p.get('current_storyboard', {}).get('content_md') or p.get('storyboard_md', '')).split())
                  for p in chapter['pages']]
        if not values or not all(values):
            raise ValueError('complete stored storyboards are required for the root draft-content audit')
        whole_draft.append({'chapter_uuid': chapter['chapter_uuid'],
                            'whole_storyboard_found': bool(values) and all(v and v in visible for v in values)})
    source_calls = []
    for item in items:
        if item.get('type') not in ('function_call', 'custom_tool_call'):
            continue
        names = re.findall(r'tools\.\w+__(chapters|render_prompt)\s*\(', item.get('input', ''))
        if item.get('name') in ('chapters', 'render_prompt'):
            names.append(item['name'])
        if names:
            source_calls.append({'call_id': item.get('call_id'), 'tools': names})
    return {'source_coverage': sources, 'draft_coverage': whole_draft,
            'source_reading_calls_to_inspect': source_calls,
            'passed': not any(s['full_source_found'] for s in sources)
                      and not any(s['whole_storyboard_found'] for s in whole_draft)}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--manifest', type=Path, required=True)
    parser.add_argument('--root-thread', required=True)
    parser.add_argument('--sessions-dir', type=Path, required=True)
    parser.add_argument('--end-at', help='ISO timestamp of the root completion; defaults to its last recorded event')
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    manifest = json.loads(args.manifest.read_text())
    project = Path(manifest['project']).resolve()
    inventory = {}
    for path in args.sessions_dir.rglob('*.jsonl'):
        meta = metadata(path)
        if meta.get('cwd') and Path(meta['cwd']).resolve() == project:
            session_id = meta['id']
            if session_id in inventory:
                raise ValueError(f'duplicate rollout for {session_id}: select one canonical history directory')
            inventory[session_id] = (path, meta)
    root_path, root_meta = inventory[args.root_thread]
    root_events = list(read_jsonl(root_path))
    start = root_meta['timestamp']
    end = args.end_at or root_events[-1]['timestamp']
    frozen_dir = args.output.parent / (args.output.stem + '-rollouts')
    frozen_dir.mkdir(parents=True, exist_ok=True)
    sessions, root_tools = [], []
    for session_id, (path, meta) in inventory.items():
        if not start <= meta['timestamp'] <= end:
            continue
        events = [e for e in read_jsonl(path) if e.get('timestamp', '') <= end]
        parent = meta.get('source', {})
        spawn = parent.get('subagent', {}).get('thread_spawn', {}) if isinstance(parent, dict) else {}
        models = sorted({(e['payload']['model'], e['payload'].get('effort')) for e in events
                         if e.get('type') == 'turn_context' and 'model' in e['payload']})
        items = tool_items(events)
        spawns = []
        for item in items:
            if item.get('type') == 'function_call' and item.get('name', '').split('.')[-1] == 'spawn_agent':
                arguments = json.loads(item['arguments'])
                spawns.append({k: v for k, v in arguments.items() if k != 'message'})
        auxiliary = isinstance(parent, dict) and bool(parent.get('subagent', {}).get('other'))
        kind = ('root' if session_id == args.root_thread else 'host_auxiliary' if auxiliary
                else 'subagent' if spawn else 'chapter')
        try:
            measured = usage(events)
        except ValueError as exc:
            raise ValueError(f'{session_id}: {exc}') from exc
        # Preserve an immutable measurement window before follow-up acceptance
        # appends image/resume turns to the live host histories.
        frozen = frozen_dir / (session_id + '.jsonl')
        with path.open('rb') as source:
            frozen.write_bytes(b''.join(line for line in source if json.loads(line).get('timestamp', '') <= end))
        sessions.append({'thread_id': session_id, 'kind': kind, 'parent_thread_id': spawn.get('parent_thread_id'),
                         'agent_path': spawn.get('agent_path'), 'thread_source': meta.get('thread_source'),
                         'models': models, 'rollout': str(frozen), 'source_rollout': str(path),
                         'rollout_sha256': hashlib.sha256(frozen.read_bytes()).hexdigest(),
                         'usage': measured, 'spawn_calls': spawns})
        if session_id == args.root_thread:
            root_tools = items
    if not sessions:
        raise ValueError('no measured sessions')
    totals = {key: sum(s['usage'][key] for s in sessions) for key in
              ('input_tokens', 'cached_input_tokens', 'uncached_input_tokens', 'output_tokens')}
    workflow_totals = {key: sum(s['usage'][key] for s in sessions if s['kind'] != 'host_auxiliary') for key in totals}
    root_usage = next(s['usage'] for s in sessions if s['thread_id'] == args.root_thread)
    snapshot = json.loads((args.manifest.parent / 'snapshot.json').read_text())
    if any(len(c['pages']) != manifest['page_count_per_chapter'] for c in snapshot['chapters']):
        raise ValueError('snapshot has not reached the common completion point')
    result = {'root_thread': args.root_thread, 'project': str(project), 'start': start, 'end': end,
              'root_single_input_peak': root_usage['single_input_peak'], 'totals': totals,
              'workflow_totals': workflow_totals, 'sessions': sessions,
              'root_content_audit': content_audit(root_tools, NOVEL, snapshot['chapters']),
              'notes': ['output_tokens already includes reasoning output; do not add it twice',
                        'cumulative counters are taken once per session; repeated events are not summed',
                        'totals include host auxiliary auto-review sessions; workflow_totals exclude only those sessions',
                        'tool outputs audited here are model-visible rollout records, not just MCP wire logs',
                        'inspect source-reading calls and draft quality alongside these exact-content checks']}
    args.output.write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps({k: v for k, v in result.items() if k not in ('sessions', 'notes')}, ensure_ascii=False))


if __name__ == '__main__':
    main()
