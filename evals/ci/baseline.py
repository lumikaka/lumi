#!/usr/bin/env python3
"""CI orchestration and descriptive comparison; never substitutes Agent responses."""
import collections
import difflib
import hashlib
import json
import math
import os
from pathlib import Path
import statistics
import subprocess
import sys
from urllib.parse import urlsplit

HERE = Path(__file__).resolve().parent
LOCK = json.loads((HERE / 'pi-source-lock.json').read_text())
SCENARIOS = {
    ('Add model to existing provider', 'adds the model without replacing existing models'): 'pi-add-model',
    ('Create a project prompt template', 'creates, reloads, and invokes /current-time'): 'pi-prompt-template-project',
    ('Create a user prompt template', 'creates, reloads, and invokes /current-time'): 'pi-prompt-template-user',
}
DIFFERENCES = [
    'Same endpoint and requested model ID; the service may route a mutable model alias to different backend revisions.',
    'The pinned pi source hydrates its ignored built-in model catalog from live upstream catalogs before building; the actual generated catalog and hashes are frozen in pi-model-catalog/.',
    'Original pi tasks/judges run against real pi resources; Lumi uses the reviewed virtual filesystem/runtime adaptation.',
    'System prompts, tool schemas/results, and available documentation differ. This is not an isolated Agent-core experiment.',
    'pi uses streaming Chat Completions; Lumi uses non-streaming Chat Completions. Both omit temperature and reasoning request fields.',
    'Both cap output at 4096 tokens. pi keeps its native 300-second test timeout and native retry/loop policy; Lumi has 300 seconds per case and 16 requests per turn, including its no-progress policy.',
    'pi errored/unscored records have no automatic Agent/environment attribution; inspect native Vitest errors and sessions. They are never silently counted as zero.',
    'No price snapshot is configured. Cost is unavailable; zero model catalog prices only disable estimation and do not mean free inference.',
    'Three repeats describe observed variation, not statistical significance. Scores do not gate infrastructure acceptance.',
]


def dump(path, value):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2, allow_nan=False) + '\n')


def sha(data):
    return hashlib.sha256(data).hexdigest()


def command(*args):
    return subprocess.check_output(args, text=True).strip()


def config(env):
    endpoint = env.get('EVAL_BASE_URL', '').rstrip('/')
    url = urlsplit(endpoint)
    if url.scheme != 'https' or not url.hostname or url.username or url.password or url.query or url.fragment:
        raise ValueError('Set a HTTPS API base URL without credentials, query, or fragment.')
    model = env.get('EVAL_MODEL', '').strip()
    if not model or len(model) > 200 or any(ord(c) < 32 for c in model):
        raise ValueError('Invalid model ID.')
    repetitions = int(env.get('EVAL_REPETITIONS', '3'))
    if not 1 <= repetitions <= 10:
        raise ValueError('Repetitions must be between 1 and 10.')
    return dict(base_url=endpoint, model=model, repetitions=repetitions, max_tokens=4096,
                temperature=None, reasoning_request_fields='omitted', timeout_seconds=300,
                pi_provider_alias='lumi-eval', lumi_provider_type='openai_compatible',
                lumi_max_requests_per_turn=16, pi_max_requests_per_turn=None)


def init(out):
    out.mkdir(parents=True, exist_ok=True)
    (out / 'disk-before.txt').write_text(command('df', '-h') + '\n')
    cfg = config(os.environ)
    if not os.environ.get('LUMI_EVAL_API_KEY'):
        raise ValueError('Configure the repository Secret LUMI_EVAL_API_KEY.')
    dump(out / 'execution.json', {
        'schema_version': 1, 'config': cfg, 'pi_revision': LOCK['revision'],
        'lumi_revision': command('git', 'rev-parse', 'HEAD'),
        'workflow_run_url': f"https://github.com/{os.environ['GITHUB_REPOSITORY']}/actions/runs/{os.environ['GITHUB_RUN_ID']}",
        'workflow_attempt': os.environ.get('GITHUB_RUN_ATTEMPT'),
        'runner_os': os.environ.get('RUNNER_OS'), 'differences': DIFFERENCES,
    })


def prepare(pi_dir, out, cfg):
    if command('git', '-C', str(pi_dir), 'rev-parse', 'HEAD') != LOCK['revision']:
        raise ValueError('Unexpected pi revision; re-review and update the source lock first.')
    for name, expected in LOCK['blobs'].items():
        if command('git', 'hash-object', str(pi_dir / name)) != expected:
            raise ValueError(f'Pinned pi source changed: {name}')
    harness_path = pi_dir / 'packages/evals/src/harness.ts'
    original = harness_path.read_text()
    anchor = '\t\tconst modelRuntime = await ModelRuntime.create({ credentials });'
    if original.count(anchor) != 1:
        raise ValueError('pi harness adapter insertion point changed.')
    adapted = 'import { configureEvaluationModel } from "./lumi-ci-model.ts";\n' + original.replace(
        anchor, anchor + '\n\t\tconfigureEvaluationModel(modelRuntime);')
    # In-memory provider registration does not pre-create models.json or alter the
    # openai provider whose configuration the task must discover and edit.
    model_config = {
        'baseUrl': cfg['base_url'], 'api': 'openai-completions',
        'models': [{'id': cfg['model'], 'name': cfg['model'], 'reasoning': True,
                    'input': ['text'], 'cost': {'input': 0, 'output': 0, 'cacheRead': 0, 'cacheWrite': 0},
                    'contextWindow': 262144, 'maxTokens': cfg['max_tokens'],
                    'compat': {'maxTokensField': 'max_tokens', 'supportsDeveloperRole': False,
                               'supportsStore': False, 'supportsReasoningEffort': False,
                               'thinkingFormat': 'openai', 'supportsUsageInStreaming': True}}],
    }
    adapter = ('import type { ModelRuntime } from "@earendil-works/pi-coding-agent";\n'
               'export function configureEvaluationModel(runtime: ModelRuntime): void {\n'
               '  runtime.registerProvider("lumi-eval", ' + json.dumps(model_config, ensure_ascii=False) + ');\n}\n')
    harness_path.write_text(adapted)
    (harness_path.parent / 'lumi-ci-model.ts').write_text(adapter)
    evidence = out / 'pi-adapter'
    evidence.mkdir(parents=True, exist_ok=True)
    (evidence / 'harness.patch').write_text(''.join(difflib.unified_diff(
        original.splitlines(True), adapted.splitlines(True), fromfile='a/packages/evals/src/harness.ts',
        tofile='b/packages/evals/src/harness.ts')))
    (evidence / 'lumi-ci-model.ts').write_text(adapter)
    docker_path = pi_dir / 'packages/evals/docker/Dockerfile'
    docker_original = docker_path.read_text()
    build_anchor = '    && npm run build:offline \\\n'
    if docker_original.count(build_anchor) != 1:
        raise ValueError('pi Docker build insertion point changed.')
    docker_adapted = docker_original.replace(build_anchor, '    && npm run hydrate:model-data \\\n' + build_anchor)
    docker_path.write_text(docker_adapted)
    (evidence / 'Dockerfile.patch').write_text(''.join(difflib.unified_diff(
        docker_original.splitlines(True), docker_adapted.splitlines(True),
        fromfile='a/packages/evals/docker/Dockerfile', tofile='b/packages/evals/docker/Dockerfile')))
    dump(evidence / 'source-lock.json', LOCK)
    dump(evidence / 'manifest.json', {
        'revision': LOCK['revision'], 'original_harness_sha256': sha(original.encode()),
        'adapted_harness_sha256': sha(adapted.encode()), 'adapter_sha256': sha(adapter.encode()),
        'model_config': model_config,
        'original_dockerfile_sha256': sha(docker_original.encode()),
        'adapted_dockerfile_sha256': sha(docker_adapted.encode()),
        'scope': 'Evaluator model registration plus official model-data hydration before offline build; no Agent core, task, tool, judge or Docker isolation edits.',
        'catalog_source': 'Upstream hydrate:model-data reads live public catalogs. The generated data and manifest are exported from the actual image and frozen with the run.',
    })


def read(path):
    return json.loads(path.read_text())


def checked_number(value, label, maximum=None):
    if isinstance(value, bool) or not isinstance(value, (float, int)) or not math.isfinite(value) or value < 0:
        raise ValueError(f'Invalid {label}')
    if maximum is not None and value > maximum:
        raise ValueError(f'Invalid {label}')
    return value


def metrics(records, field):
    values = [checked_number(r[field], field) for r in records if r.get(field) is not None]
    return {'available_runs': len(values), 'total': sum(values) if values else None,
            'mean': statistics.mean(values) if values else None,
            'min': min(values) if values else None, 'max': max(values) if values else None,
            'stddev': statistics.pstdev(values) if values else None}


def summarize(records, planned):
    scores = [checked_number(r['score'], 'score', 1) for r in records if r.get('score') is not None]
    successes = sum(v == 1 for v in scores)
    return {'planned': planned, 'recorded': len(records), 'scored': len(scores),
            'successes': successes, 'pass_rate': successes / len(scores) if scores else None,
            'statuses': dict(collections.Counter(r['status'] for r in records)),
            'score': metrics(records, 'score'), 'duration_ms': metrics(records, 'duration_ms'),
            'input_tokens': metrics(records, 'input_tokens'), 'output_tokens': metrics(records, 'output_tokens'),
            'cached_input_tokens': metrics(records, 'cached_input_tokens'),
            'tool_calls': metrics(records, 'tool_calls'), 'cost_usd': metrics(records, 'cost_usd'),
            'runs': records}


def pi_identity(item):
    return tuple(item[k] for k in ('evalSet', 'caseId', 'variant', 'model', 'runNumber'))


def load_pi(root, cfg):
    candidates = list(root.glob('*/protocol.json'))
    if len(candidates) != 1:
        raise ValueError(f'Expected exactly one pi cohort, found {len(candidates)}.')
    directory = candidates[0].parent
    protocol, expected = read(directory / 'protocol.json'), read(directory / 'expected-runs.json')
    if protocol.get('schemaVersion') != 1 or protocol.get('runsPerVariant') != cfg['repetitions']:
        raise ValueError('pi protocol schema or repetition count mismatch.')
    protocol_copy = dict(protocol)
    digest = protocol_copy.pop('protocolDigest')
    if sha(json.dumps(protocol_copy, ensure_ascii=False, separators=(',', ':')).encode()) != digest:
        raise ValueError('pi protocol digest mismatch.')
    if protocol['tasks'] != expected or protocol['model'] != 'lumi-eval/' + cfg['model']:
        raise ValueError('pi protocol/plan/model mismatch.')
    planned = {(*scenario, variant, 'lumi-eval/' + cfg['model'], repeat)
               for scenario in SCENARIOS for variant in ('with_docs', 'without_docs')
               for repeat in range(1, cfg['repetitions'] + 1)}
    identities = [pi_identity(t) for t in expected]
    if len(identities) != len(set(identities)) or set(identities) != planned:
        raise ValueError('pi expected cohort is missing, duplicated, or unexpected.')
    observations = [json.loads(line) for line in (directory / 'observations.jsonl').read_text().splitlines() if line]
    observed = [pi_identity(t) for t in observations]
    if len(observed) != len(set(observed)) or not set(observed).issubset(planned):
        raise ValueError('pi observations contain duplicates or unexpected tasks.')
    blocked = []
    if set(observed) != planned:
        blocked.append(f'pi missing {len(planned - set(observed))} observations')
    report = read(directory / 'report.json')
    if report.get('schemaVersion') != 3 or report.get('protocolDigest') != digest:
        raise ValueError('pi report schema/digest mismatch.')
    if report['blockedPairs']:
        blocked.append(f"pi native report blocked {len(report['blockedPairs'])} pairs; inspect raw errors")
    rows = collections.defaultdict(list)
    for item in observations:
        status = item['outcome']
        if status not in ('scored', 'unscored', 'skipped', 'pending', 'errored'):
            raise ValueError('Unknown pi outcome.')
        if status != 'scored':
            blocked.append(f"pi {item['evalSet']}/{item['variant']}/{item['runNumber']}: {status}, attribution unavailable")
        slug = SCENARIOS[(item['evalSet'], item['caseId'])] + '-' + item['variant'].replace('_', '-')
        task_key = json.dumps(list(pi_identity(item)), ensure_ascii=False, separators=(',', ':'))
        task_dir = directory / 'tasks' / sha(task_key.encode())
        if not (task_dir / 'vitest.json').is_file():
            blocked.append(f'{slug}/{item["runNumber"]}: missing pi Vitest evidence')
        session = directory / item['variant'] / 'sessions' / sha(task_key.encode()) / 'session.jsonl'
        if status == 'scored' and not session.is_file():
            blocked.append(f'{slug}/{item["runNumber"]}: missing pi session evidence')
        rows[slug].append({
            'repetition': item['runNumber'], 'status': status,
            'score': checked_number(item.get('score'), 'pi score', 1) if status == 'scored' else None,
            'duration_ms': item.get('totalMs'), 'input_tokens': item.get('inputTokens'),
            'output_tokens': item.get('outputTokens'), 'cached_input_tokens': item.get('cacheReadTokens'),
            'tool_calls': item.get('toolCalls'), 'cost_usd': item.get('estimatedCostUsd'),
            'evidence': str(task_dir.relative_to(root.parent)),
            'session': str(session.relative_to(root.parent)) if session.is_file() else None,
        })
    return rows, blocked, str(directory.relative_to(root.parent))


def load_lumi(root, cfg):
    report, protocol = read(root / 'report.json'), read(root / 'protocol.json')
    if report.get('schema_version') != 1 or protocol.get('schema_version') != 1:
        raise ValueError('Lumi report/protocol schema mismatch.')
    if report['protocol_digest'] != protocol['digest']:
        raise ValueError('Lumi protocol/report mismatch.')
    for key in ('base_url', 'model', 'repetitions', 'max_tokens', 'temperature'):
        if report['config'][key] != cfg[key]:
            raise ValueError(f'Lumi configuration mismatch: {key}')
    if (report['config']['timeout_seconds_per_case'] != cfg['timeout_seconds'] or
            report['config']['max_requests_per_turn'] != cfg['lumi_max_requests_per_turn'] or
            report['config']['provider_type'] != cfg['lumi_provider_type']):
        raise ValueError('Lumi timeout/request/provider configuration mismatch.')
    planned = {(slug + '-' + variant, repeat) for slug in SCENARIOS.values()
               for variant in ('with-docs', 'without-docs') for repeat in range(1, cfg['repetitions'] + 1)}
    expected = [(r['case_slug'], r['repetition']) for r in protocol['tasks']]
    if len(expected) != len(planned) or set(expected) != planned:
        raise ValueError('Lumi planned cohort mismatch.')
    rows, seen, blocked = collections.defaultdict(list), set(), list(report['blocked'])
    for item in report['results']:
        task = item['task']
        identity = (task['case_slug'], task['repetition'])
        if identity in seen or identity not in planned:
            raise ValueError('Lumi duplicate or unexpected result.')
        seen.add(identity)
        directory = root / 'tasks' / task['case_slug'] / f"run-{task['repetition']:03d}"
        if read(directory / 'result.json') != item or sha((directory / 'trace.jsonl').read_bytes()) != item['trace_sha256']:
            raise ValueError('Lumi result/trace evidence mismatch.')
        status = item['status']
        if status not in ('scored', 'agent_error', 'environment_error'):
            raise ValueError('Unknown Lumi outcome.')
        if status == 'environment_error':
            blocked.append(f'Lumi {identity}: environment_error')
        scores = item['scores']
        if status != 'environment_error' and (len(scores) != 1 or scores[0]['name'] != 'upstream_contract'):
            raise ValueError('Unexpected Lumi grading contract.')
        score = checked_number(scores[0]['value'], 'Lumi score', 1) if status != 'environment_error' else None
        rows[task['case_slug']].append({
            'repetition': task['repetition'], 'status': status, 'score': score,
            'duration_ms': item['duration_ms'], 'input_tokens': item['usage']['input_tokens'],
            'output_tokens': item['usage']['output_tokens'], 'cached_input_tokens': item['usage']['cached_input_tokens'],
            'tool_calls': len(item.get('tools') or []), 'cost_usd': item['usage']['cost_usd'],
            'issues': item['issues'], 'evidence': str(directory.relative_to(root.parent)),
        })
    if seen != planned:
        blocked.append(f'Lumi missing {len(planned - seen)} results')
    return rows, blocked


def report(out):
    blocked, pi, lumi, pi_path = [], {}, {}, None
    try:
        execution = read(out / 'execution.json')
        cfg = execution['config']
    except (OSError, ValueError, KeyError) as error:
        execution, cfg = {}, {'repetitions': 0}
        blocked.append(f'Execution metadata unavailable: {error}')
    for engine in ('pi', 'lumi'):
        try:
            if engine == 'pi':
                pi, errors, pi_path = load_pi(out / 'pi', cfg)
            else:
                lumi, errors = load_lumi(out / 'lumi', cfg)
            blocked.extend(errors)
        except (OSError, ValueError, KeyError, TypeError) as error:
            blocked.append(f'{engine} evidence unavailable or invalid: {error}')
        try:
            code = int((out / f'{engine}-exit-code.txt').read_text())
            if code:
                blocked.append(f'{engine} runner exit status {code}; inspect console and native results')
        except (OSError, ValueError):
            blocked.append(f'{engine} runner exit status unavailable')
    rows = []
    for prefix in SCENARIOS.values():
        for variant in ('with-docs', 'without-docs'):
            slug = prefix + '-' + variant
            rows.append({'case': slug, 'comparison': 'descriptive_adapted', 'delta_percentage_points': None,
                         'lumi': summarize(lumi.get(slug, []), cfg['repetitions']),
                         'pi': summarize(pi.get(slug, []), cfg['repetitions'])})
    result = {'schema_version': 1, 'execution': execution, 'blocked': blocked,
              'differences': DIFFERENCES, 'rows': rows}
    dump(out / 'comparison.json', result)
    lines = ['# Lumi / pi remote baseline', '',
             f"Requested model: `{cfg.get('model', 'N/A')}`. Repeats per variant: {cfg['repetitions']}.", '',
             'Descriptive comparison of adapted tasks. A matching model ID does not establish Agent-core equivalence.', '',
             '| Case | Lumi success / scored (planned) | pi success / scored (planned) |',
             '|---|---:|---:|']
    for row in rows:
        def cell(summary):
            rate = f"{summary['pass_rate']:.1%}" if summary['pass_rate'] is not None else 'N/A'
            return f"{summary['successes']}/{summary['scored']} ({summary['planned']}); {rate}"
        lines.append(f"| {row['case']} | {cell(row['lumi'])} | {cell(row['pi'])} |")
    lines += ['', 'Score, duration and token mean/min/max/stddev, metric availability, per-run statuses and evidence paths are in [comparison.json](comparison.json).', '',
              '[Lumi native report](lumi/report.md).']
    if pi_path:
        lines += [f'[pi native report]({pi_path}/report.txt). Native sessions and Vitest errors remain alongside it.']
    lines += ['', '## Evidence / infrastructure issues', '']
    lines += ['- ' + item for item in blocked] or ['None. Low scores are retained and do not fail this workflow.']
    lines += ['', '## Interpretation and remaining differences', ''] + ['- ' + item for item in DIFFERENCES]
    (out / 'comparison.md').write_text('\n'.join(lines) + '\n')
    summary = os.environ.get('GITHUB_STEP_SUMMARY')
    if summary:
        with open(summary, 'a') as stream:
            stream.write('\n'.join(lines) + '\n\nDownload the artifact to use the relative evidence links.\n')
    return result


def seal(out, key):
    files = sorted(p for p in out.rglob('*') if p.is_file())
    for path in out.rglob('*'):
        if path.is_symlink():
            raise ValueError('Refusing artifact with symlinks.')
    encoded_key = key.encode() if key else None
    for path in files:
        if path.name == 'auth.json' or (encoded_key and encoded_key in path.read_bytes()):
            raise ValueError('Credential material detected; artifact upload blocked. Raw evidence remains on the ephemeral runner.')
    dump(out / 'sha256-manifest.json', {str(p.relative_to(out)): sha(p.read_bytes())
                                      for p in files if p.name != 'sha256-manifest.json'})


def verify(out):
    out = out.resolve()
    manifest = read(out / 'sha256-manifest.json')
    actual = {str(p.relative_to(out)) for p in out.rglob('*') if p.is_file()
              and p != out / 'sha256-manifest.json'}
    if set(manifest) != actual:
        raise ValueError('Artifact file inventory differs from the sealed manifest.')
    for name, digest in manifest.items():
        path = out / name
        if Path(name).is_absolute() or '..' in Path(name).parts or not path.resolve().is_relative_to(out):
            raise ValueError('Unsafe artifact manifest path.')
        if any(p.is_symlink() for p in [path, *path.parents]):
            raise ValueError('Artifact manifest points through a symlink.')
        if sha(path.read_bytes()) != digest:
            raise ValueError(f'Artifact digest mismatch: {name}')
    return len(manifest)


def main():
    action = sys.argv[1]
    out = Path(os.environ['OUT'])
    if action == 'init':
        init(out)
    elif action == 'prepare':
        prepare(Path(os.environ['PI_DIR']), out, config(os.environ))
    elif action == 'report':
        report(out)
    elif action == 'seal':
        seal(out, os.environ.get('LUMI_EVAL_API_KEY', ''))
    elif action == 'verify':
        print(f'Verified {verify(out)} artifact files.')
    elif action == 'status':
        result = read(out / 'comparison.json')
        if result['blocked']:
            print('Evidence/infrastructure incomplete; see saved comparison and native logs.')
            return 1
    else:
        raise ValueError('Unknown action')
    return 0


if __name__ == '__main__':
    try:
        sys.exit(main())
    except (OSError, ValueError, KeyError, subprocess.CalledProcessError) as error:
        print(f'Baseline infrastructure error: {error}', file=sys.stderr)
        sys.exit(1)
