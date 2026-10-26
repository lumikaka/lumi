"""Synthetic evidence tests only; none of these results claim real Agent performance."""
import copy
import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import baseline as evidence
import shared


class SharedTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.out = Path(self.temp.name)
        env = patch.dict(os.environ, {'GITHUB_STEP_SUMMARY': ''})
        env.start()
        self.addCleanup(env.stop)
        cases = [evidence.read(evidence.HERE.parent.parent / 'internal/agentevals/cases' / (s + '.json')) for s in shared.SLUGS]
        cfg = dict(base_url='https://example.invalid/v1', model='test-only', repetitions=2, max_tokens=4096,
                   temperature=None, max_requests_per_turn=16, timeout_seconds=300)
        tasks = [dict(case_slug=s, repetition=r) for r in (1, 2) for s in shared.SLUGS]
        self.protocol = dict(schema_version=1, config=cfg, tasks=tasks, pi_revision='test-only',
                             spec=dict(cases=cases, system_prompt='test-only'))
        evidence.dump(self.out / 'shared-protocol.json', self.protocol)
        evidence.dump(self.out / 'shared-expected-runs.json', tasks)
        evidence.dump(self.out / 'execution.json', {'config': cfg})
        evidence.dump(self.out / 'pi-runtime/manifest.json', {'revision': 'test-only'})
        native = dict(cases=cases, system_prompt='test-only', tasks=tasks, digest='test-only',
                      config=dict(cfg, timeout_seconds_per_case=300))
        evidence.dump(self.out / 'lumi/protocol.json', native)
        for engine in ('lumi', 'pi'):
            for task in tasks:
                directory = self.out / engine / 'tasks' / task['case_slug'] / f"run-{task['repetition']:03d}"
                directory.mkdir(parents=True)
                trace = b'{"synthetic_test":true}\n'
                (directory / 'trace.jsonl').write_bytes(trace)
                case = next(c for c in cases if c['slug'] == task['case_slug'])
                item = dict(task=task, protocol_digest='test-only' if engine == 'lumi' else evidence.sha((self.out / 'shared-protocol.json').read_bytes()),
                            status='scored', scores=[dict(name=c['name'], value=0) for c in case['checks']],
                            trace_sha256=evidence.sha(trace), issues=None, usage=dict(input_tokens=None, output_tokens=None,
                            cached_input_tokens=None, cost_usd=None), duration_ms=10)
                evidence.dump(directory / 'result.json', item)

    def item_path(self, engine='pi'):
        return self.out / engine / 'tasks/smoke/run-001/result.json'

    def test_complete_low_scores_do_not_block(self):
        self.assertFalse(shared.report(self.out))
        report = evidence.read(self.out / 'shared-comparison.json')
        self.assertEqual(len(report['dimensions']), 13)
        self.assertTrue(all(r['observed_delta'] == 0 for r in report['dimensions']))
        self.assertIsNone(report['metrics']['pi']['smoke']['input_tokens']['total'])

    def test_missing_trace_or_record_blocks_comparison(self):
        self.item_path().with_name('trace.jsonl').write_text('changed')
        self.assertTrue(shared.report(self.out))
        self.item_path().unlink()
        self.assertTrue(shared.report(self.out))
        report = evidence.read(self.out / 'shared-comparison.json')
        self.assertIsNone(next(r for r in report['dimensions'] if r['case'] == 'smoke')['observed_delta'])

    def test_environment_failure_is_unavailable_not_zero(self):
        path = self.item_path()
        item = evidence.read(path)
        item.update(status='environment_error', scores=None, issues=[dict(code='timeout', message='test')])
        evidence.dump(path, item)
        self.assertTrue(shared.report(self.out))
        report = evidence.read(self.out / 'shared-comparison.json')
        row = next(r for r in report['dimensions'] if r['case'] == 'smoke')
        self.assertEqual(row['pi']['available_runs'], 1)
        self.assertIsNone(row['observed_delta'])

    def test_agent_failure_keeps_real_dimensions(self):
        path = self.item_path()
        item = evidence.read(path)
        item['status'] = 'agent_error'
        evidence.dump(path, item)
        self.assertFalse(shared.report(self.out))

    def test_schema_model_and_plan_drift_block(self):
        original = evidence.read(self.out / 'lumi/protocol.json')
        for mutate in [lambda p: p['config'].update(model='other-model'),
                       lambda p: p['cases'][0]['steps'][0].update(content='different task'),
                       lambda p: p['tasks'].append(p['tasks'][0])]:
            changed = copy.deepcopy(original)
            mutate(changed)
            evidence.dump(self.out / 'lumi/protocol.json', changed)
            self.assertTrue(shared.report(self.out))

    def test_duplicate_scores_and_unexpected_runs_block(self):
        path = self.item_path()
        item = evidence.read(path)
        item['scores'] *= 2
        evidence.dump(path, item)
        self.assertTrue(shared.report(self.out))
        item['scores'] = item['scores'][:1]
        item['task']['repetition'] = 99
        evidence.dump(path, item)
        self.assertTrue(shared.report(self.out))


if __name__ == '__main__':
    unittest.main()
