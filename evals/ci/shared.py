"""Controlled shared-task comparison. Reports describe observations, not causal ranking."""
import json
import os
from pathlib import Path
import subprocess
import sys
import time

import baseline as evidence

SLUGS = ['audit-inconclusive', 'audit-match', 'audit-mismatch', 'smoke']
LIMITATIONS = [
    'This is a shared generic Agent-core benchmark, not the original pi product-task suite or a full application comparison.',
    'The task, fixtures, fixed system prompt, tool definitions, Go tool implementations and Go grading code are shared.',
    'pi uses its real Agent with streaming pi-ai; Lumi uses its real durable Agent with non-streaming requests. Native validation, context assembly, repair, retry and finalization policies remain different.',
    'Both request the same endpoint/model, omit temperature/reasoning fields, and cap output at 4096 tokens and logical model requests at 16 per turn. Provider-level retries can issue additional HTTP attempts.',
    'The model ID is a mutable provider alias. Engines run sequentially and are not a randomized causal experiment.',
    'Input/output token totals use provider prompt_tokens/completion_tokens, including cache and any reported reasoning tokens. Missing metrics and all costs are unavailable, never zero-filled.',
    'Audit evidence scoring only checks one final submission citing src/names.py and format_name; it does not prove the explanation is semantically correct.',
    'Small repeat counts show observed variation only. Low scores do not fail CI; incomplete or environment-failed evidence does.',
]


def plan(out):
    cfg = evidence.read(out / 'execution.json')['config']
    binary = os.environ['LUMI_EVAL_BINARY']
    subprocess.run([binary, '--export-shared', str(out / 'shared-spec.json')], check=True)
    spec = evidence.read(out / 'shared-spec.json')
    if [c['slug'] for c in spec['cases']] != SLUGS:
        raise ValueError('Unexpected shared case cohort')
    config = dict(cfg, max_requests_per_turn=16)
    tasks = [dict(case_slug=slug, repetition=repeat) for repeat in range(1, cfg['repetitions'] + 1)
             for slug in (SLUGS if repeat % 2 else list(reversed(SLUGS)))]
    evidence.dump(out / 'shared-protocol.json', {
        'schema_version': 1, 'spec': spec, 'config': config, 'tasks': tasks,
        'pi_revision': evidence.read(evidence.HERE / 'pi-runtime-lock.json')['revision'],
        'lumi_revision': evidence.command('git', 'rev-parse', 'HEAD'), 'limitations': LIMITATIONS,
    })
    evidence.dump(out / 'shared-expected-runs.json', tasks)


def run_pi(out):
    protocol_path = out / 'shared-protocol.json'
    protocol = evidence.read(protocol_path)
    digest = evidence.sha(protocol_path.read_bytes())
    for task in protocol['tasks']:
        directory = out / 'pi/tasks' / task['case_slug'] / f"run-{task['repetition']:03d}"
        directory.mkdir(parents=True, exist_ok=False)
        started = time.time()
        try:
            with (directory / 'console.txt').open('w') as console:
                result = subprocess.run(['node', str(evidence.HERE / 'pi-shared.mjs'), str(protocol_path),
                    task['case_slug'], str(task['repetition']), str(directory)], stdout=console,
                    stderr=subprocess.STDOUT, timeout=protocol['config']['timeout_seconds'] + 15)
            failure = f'pi adapter exited {result.returncode}' if result.returncode else None
        except subprocess.TimeoutExpired:
            failure = 'pi adapter exceeded hard timeout'
        if not (directory / 'result.json').is_file():
            # An interrupted process never gets a fabricated Agent score. Keep partial traces.
            trace = directory / 'trace.jsonl'
            with trace.open('a') as stream:
                stream.write(json.dumps({'type': 'environment_error', 'data': failure or 'Missing pi result'}) + '\n')
            evidence.dump(directory / 'result.json', {'task': task, 'protocol_digest': digest,
                'status': 'environment_error', 'scores': None, 'final': '', 'tools': None, 'files': None,
                'issues': [{'kind': 'environment', 'code': 'adapter_process', 'message': failure or 'Missing pi result'}],
                'duration_ms': round((time.time() - started) * 1000), 'usage': {
                    'requests': None, 'input_tokens': None, 'output_tokens': None, 'cached_input_tokens': None, 'cost_usd': None},
                'trace_sha256': evidence.sha(trace.read_bytes())})
        elif failure and evidence.read(directory / 'result.json')['status'] != 'environment_error':
            raise ValueError('pi process failed after producing a non-error result')
        record = evidence.read(directory / 'result.json')
        print(f"pi {task['case_slug']}/run-{task['repetition']:03d}: {record['status']}", flush=True)


def identity(task):
    return task['case_slug'], task['repetition']


def load_results(out, engine, protocol):
    expected = {identity(t) for t in protocol['tasks']}
    definitions = {c['slug']: c for c in protocol['spec']['cases']}
    root = out / engine
    if engine == 'lumi':
        native = evidence.read(root / 'protocol.json')
        actual_cases = {c['slug']: c for c in native['cases'] if c['slug'] in SLUGS}
        if actual_cases != definitions or native['system_prompt'] != protocol['spec']['system_prompt']:
            raise ValueError('Lumi task or fixed system prompt drift')
        for key in ('base_url', 'model', 'repetitions', 'max_tokens', 'temperature', 'max_requests_per_turn'):
            if native['config'][key] != protocol['config'][key]:
                raise ValueError(f'Lumi configuration drift: {key}')
        if native['config']['timeout_seconds_per_case'] != protocol['config']['timeout_seconds']:
            raise ValueError('Lumi timeout drift')
        planned = [identity(t) for t in native['tasks'] if t['case_slug'] in SLUGS]
        if len(planned) != len(expected) or set(planned) != expected:
            raise ValueError('Lumi shared plan is incomplete or duplicated')
        digest = native['digest']
    else:
        if evidence.read(out / 'pi-runtime/manifest.json')['revision'] != protocol['pi_revision']:
            raise ValueError('pi runtime source revision mismatch')
        digest = evidence.sha((out / 'shared-protocol.json').read_bytes())
    rows, seen, blocked = {s: [] for s in SLUGS}, set(), []
    for file in sorted((root / 'tasks').glob('*/run-*/result.json')):
        item = evidence.read(file)
        key = identity(item['task'])
        if engine == 'lumi' and key[0] not in SLUGS:
            continue
        if key not in expected or key in seen:
            raise ValueError(f'{engine}: duplicate or unexpected result {key}')
        seen.add(key)
        if item['protocol_digest'] != digest or evidence.sha(file.with_name('trace.jsonl').read_bytes()) != item['trace_sha256']:
            raise ValueError(f'{engine}: protocol or trace hash mismatch {key}')
        if item['status'] not in ('scored', 'agent_error', 'environment_error'):
            raise ValueError(f'{engine}: unknown status')
        scores = item['scores']
        if item['status'] == 'environment_error':
            if scores is not None: raise ValueError('Environment failures cannot be scored')
            blocked.append(f'{engine}/{key[0]}/{key[1]}: environment_error')
        else:
            names = [s['name'] for s in scores]
            expected_names = {s['name'] for s in definitions[key[0]]['checks']}
            if len(names) != len(expected_names) or set(names) != expected_names:
                raise ValueError(f'{engine}: missing or duplicate grading dimensions')
            for score in scores: evidence.checked_number(score['value'], 'score', 1)
        rows[key[0]].append({**item, 'evidence': str(file.relative_to(out))})
    if seen != expected:
        blocked.append(f'{engine}: missing {len(expected - seen)} planned results')
    return rows, blocked


def report(out):
    blocked, rows, protocol = [], {}, {}
    try:
        protocol = evidence.read(out / 'shared-protocol.json')
        if protocol['schema_version'] != 1 or [c['slug'] for c in protocol['spec']['cases']] != SLUGS:
            raise ValueError('Invalid shared specification')
        cfg = protocol['config']
        expected = {(s, r) for s in SLUGS for r in range(1, cfg['repetitions'] + 1)}
        plan_items = [identity(t) for t in protocol['tasks']]
        if (len(plan_items) != len(expected) or set(plan_items) != expected or
                evidence.read(out / 'shared-expected-runs.json') != protocol['tasks']):
            raise ValueError('Shared planned cohort mismatch')
        execution = evidence.read(out / 'execution.json')
        if any(cfg[k] != v for k, v in execution['config'].items()):
            raise ValueError('Shared execution configuration mismatch')
        for engine in ('lumi', 'pi'):
            try:
                rows[engine], issues = load_results(out, engine, protocol)
                blocked.extend(issues)
            except (OSError, KeyError, ValueError, TypeError) as error:
                blocked.append(f'{engine} evidence unavailable or invalid: {error}')
    except (OSError, KeyError, ValueError, TypeError) as error:
        blocked.append(f'Shared protocol unavailable or invalid: {error}')
    comparisons = []
    lines = ['# Lumi / pi 共有任务对照', '', '比较范围：真实 Agent 核心；4 个通用任务。每项 0–1，1 表示该项观察条件满足，无统一通过门槛。', '',
             '| 用例 / 分项 | Lumi 均值（有效/计划） | pi 均值（有效/计划） | Lumi − pi |', '|---|---:|---:|---:|']
    for case in protocol.get('spec', {}).get('cases', []):
        for check in case['checks']:
            row = {'case': case['slug'], 'dimension': check['name']}
            for engine in ('lumi', 'pi'):
                records = rows.get(engine, {}).get(case['slug'], [])
                values = [{'score': s['value']} for r in records if r['status'] != 'environment_error'
                          for s in r['scores'] if s['name'] == check['name']]
                row[engine] = evidence.metrics(values, 'score')
            complete = all(row[e]['available_runs'] == protocol['config']['repetitions'] for e in ('lumi', 'pi'))
            row['observed_delta'] = row['lumi']['mean'] - row['pi']['mean'] if complete else None
            def cell(engine):
                data = row[engine]
                return f"{data['mean']:.3f} ({data['available_runs']}/{protocol['config']['repetitions']})" if data['mean'] is not None else 'N/A'
            delta = f"{row['observed_delta']:+.3f}" if complete else 'N/A'
            lines.append(f"| {case['slug']} / {check['name']} | {cell('lumi')} | {cell('pi')} | {delta} |")
            comparisons.append(row)
    lines += ['', '## 执行与波动', '', '| 引擎 / 用例 | 状态 | 耗时均值/范围/标准差（秒） | 输入 / 输出 Token 总计 |', '|---|---|---|---|']
    summaries = {}
    for engine, cases in rows.items():
        summaries[engine] = {}
        for slug, records in cases.items():
            flat = [{**r, **r['usage']} for r in records]
            stats = {field: evidence.metrics(flat, field) for field in ('duration_ms', 'input_tokens', 'output_tokens', 'cached_input_tokens', 'cost_usd')}
            summaries[engine][slug] = stats
            timing = stats['duration_ms']
            duration = (f"{timing['mean']/1000:.2f} / {timing['min']/1000:.2f}–{timing['max']/1000:.2f} / {timing['stddev']/1000:.2f}" if timing['mean'] is not None else 'N/A')
            tokens = ' / '.join(str(stats[k]['total']) if stats[k]['available_runs'] == len(records) and records else 'N/A' for k in ('input_tokens','output_tokens'))
            lines.append(f"| {engine} / {slug} | {', '.join(r['status'] for r in records)} | {duration} | {tokens} |")
    lines += ['', '## 每次执行', '']
    for engine, cases in rows.items():
        for slug, records in cases.items():
            for r in records:
                lines.append(f"- [{engine}/{slug}/run-{r['task']['repetition']:03d}]({r['evidence']}): {r['status']}; " +
                             '; '.join(i['code'] + ': ' + i['message'] for i in (r['issues'] or [])))
    lines += ['', '## 解释边界', ''] + ['- ' + s for s in LIMITATIONS]
    if blocked: lines += ['', '## 证据问题', ''] + ['- ' + issue for issue in blocked]
    evidence.dump(out / 'shared-comparison.json', {'schema_version': 1, 'blocked': blocked,
        'protocol': protocol, 'dimensions': comparisons, 'metrics': summaries, 'results': rows, 'limitations': LIMITATIONS})
    (out / 'shared-comparison.md').write_text('\n'.join(lines) + '\n')
    if os.environ.get('GITHUB_STEP_SUMMARY'):
        with open(os.environ['GITHUB_STEP_SUMMARY'], 'a') as stream: stream.write('\n'.join(lines) + '\n')
    return blocked


if __name__ == '__main__':
    out, action = Path(os.environ['OUT']), sys.argv[1]
    if action == 'plan': plan(out)
    elif action == 'run-pi': run_pi(out)
    elif action == 'report': report(out)
    elif action == 'status':
        sys.exit(int(bool(evidence.read(out / 'shared-comparison.json')['blocked'] or evidence.read(out / 'status.json')['blocked'])))
    else: raise ValueError('Unknown shared benchmark action')
