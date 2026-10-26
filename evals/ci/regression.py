#!/usr/bin/env python3
"""Manual CI metadata and completeness checks for the general Lumi cohort."""
import os
from pathlib import Path
import sys

import baseline as evidence


def init(out):
    cfg = evidence.config(os.environ)
    if not os.environ.get('LUMI_EVAL_API_KEY'):
        raise ValueError('Configure the repository Secret LUMI_EVAL_API_KEY.')
    cases = [evidence.read(p) for p in sorted((evidence.HERE.parent.parent / 'internal/agentevals/cases').glob('*.json'))]
    if not cases or len({c['slug'] for c in cases}) != len(cases):
        raise ValueError('Empty or duplicate case catalog.')
    # Shared execution settings; the pi adapter adds its explicit request limit.
    cfg = {k: cfg[k] for k in ('base_url', 'model', 'repetitions', 'max_tokens', 'temperature',
                              'reasoning_request_fields', 'timeout_seconds', 'lumi_max_requests_per_turn')}
    evidence.dump(out / 'execution.json', {
        'schema_version': 1, 'suite': 'regression', 'config': cfg,
        'case_slugs': [c['slug'] for c in cases],
        'lumi_revision': evidence.command('git', 'rev-parse', 'HEAD'),
        'workflow_run_url': f"https://github.com/{os.environ['GITHUB_REPOSITORY']}/actions/runs/{os.environ['GITHUB_RUN_ID']}",
        'workflow_attempt': os.environ.get('GITHUB_RUN_ATTEMPT'),
        'pi_comparison': 'shared-comparison.json (smoke and three document audits)',
    })


def report(out):
    blocked = []
    try:
        execution = evidence.read(out / 'execution.json')
        cfg = execution['config']
        result = evidence.read(out / 'lumi/report.json')
        protocol = evidence.read(out / 'lumi/protocol.json')
        if result['schema_version'] != 1 or protocol['schema_version'] != 1 or result['protocol_digest'] != protocol['digest']:
            raise ValueError('Lumi report/protocol mismatch.')
        for field in ('base_url', 'model', 'repetitions', 'max_tokens', 'temperature'):
            if result['config'][field] != cfg[field]:
                raise ValueError(f'Lumi configuration mismatch: {field}')
        if (result['config']['provider_type'] != 'openai_compatible' or
                result['config']['timeout_seconds_per_case'] != cfg['timeout_seconds'] or
                result['config']['max_requests_per_turn'] != cfg['lumi_max_requests_per_turn']):
            raise ValueError('Lumi provider, timeout or request budget mismatch.')
        expected = {(slug, repeat) for slug in execution['case_slugs'] for repeat in range(1, cfg['repetitions'] + 1)}
        planned = [(t['case_slug'], t['repetition']) for t in protocol['tasks']]
        actual = [(r['task']['case_slug'], r['task']['repetition']) for r in result['results']]
        if len(planned) != len(expected) or set(planned) != expected:
            raise ValueError('Lumi planned cohort mismatch.')
        if len(actual) != len(expected) or set(actual) != expected:
            blocked.append('Lumi results are missing, duplicated or unexpected.')
        blocked.extend(result['blocked'])
        if any(r['status'] not in ('scored', 'agent_error') for r in result['results']):
            blocked.append('Lumi has environment errors or unknown statuses.')
        if int((out / 'lumi-exit-code.txt').read_text()) != 0:
            blocked.append('Lumi runner failed; inspect console and task records.')
    except (OSError, ValueError, KeyError, TypeError) as error:
        blocked.append(f'Lumi evidence unavailable or invalid: {error}')
    evidence.dump(out / 'status.json', {'schema_version': 1, 'blocked': blocked, 'pi_comparison': 'shared-comparison.json'})
    summary = os.environ.get('GITHUB_STEP_SUMMARY')
    if summary:
        with open(summary, 'a') as stream:
            stream.write('# Lumi Agent regression\n\nEight general cases; four also run on pi. See the shared comparison and full artifact.\n\n')
            if (out / 'lumi/report.md').is_file():
                stream.write((out / 'lumi/report.md').read_text())
            if blocked:
                stream.write('\n\n## Evidence issues\n\n' + '\n'.join('- ' + issue for issue in blocked))
    return blocked


def main():
    action = sys.argv[1]
    out = Path(os.environ['OUT'])
    if action == 'init':
        init(out)
    elif action == 'report':
        report(out)
    elif action == 'status':
        return int(bool(evidence.read(out / 'status.json')['blocked']))
    else:
        raise ValueError('Unknown action.')
    return 0


if __name__ == '__main__':
    try:
        sys.exit(main())
    except (OSError, ValueError, KeyError) as error:
        print(f'Eval infrastructure error: {error}', file=sys.stderr)
        sys.exit(1)
