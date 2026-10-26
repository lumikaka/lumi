"""Offline CI completeness tests; these fixtures are not real-model evidence."""
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import baseline as evidence
import regression


class RegressionTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.out = Path(self.temp.name)
        self.env = patch.dict(os.environ, {
            'OUT': str(self.out), 'EVAL_MODEL': 'unit-test-model',
            'EVAL_BASE_URL': 'https://example.invalid/v1', 'EVAL_REPETITIONS': '2',
            'LUMI_EVAL_API_KEY': 'unit-test-key', 'GITHUB_REPOSITORY': 'example/test',
            'GITHUB_RUN_ID': 'unit-test-only', 'GITHUB_STEP_SUMMARY': '',
        })
        self.env.start()
        self.addCleanup(self.env.stop)

    def fixture(self):
        regression.init(self.out)
        execution = evidence.read(self.out / 'execution.json')
        config = dict(execution['config'], provider_type='openai_compatible',
                      timeout_seconds_per_case=300, max_requests_per_turn=16)
        tasks = [dict(case_slug=slug, repetition=repeat) for slug in execution['case_slugs'] for repeat in (1, 2)]
        evidence.dump(self.out / 'lumi/protocol.json', {'schema_version': 1, 'digest': 'unit-test-only', 'tasks': tasks})
        result = {'schema_version': 1, 'protocol_digest': 'unit-test-only', 'config': config, 'blocked': [],
                  'results': [dict(task=t, status='scored', scores=[dict(value=0)]) for t in tasks]}
        evidence.dump(self.out / 'lumi/report.json', result)
        (self.out / 'lumi-exit-code.txt').write_text('0\n')
        return execution, result

    def test_eight_remaining_cases_and_low_scores_do_not_fail_ci(self):
        execution, _ = self.fixture()
        self.assertEqual(execution['case_slugs'], ['audit-inconclusive', 'audit-match', 'audit-mismatch',
            'batch-task', 'context-handoff', 'missing-resource', 'smoke', 'transient-tool'])
        self.assertNotIn('unit-test-key', (self.out / 'execution.json').read_text())
        self.assertIn('shared-comparison.json', execution['pi_comparison'])
        self.assertFalse(regression.report(self.out))
        self.assertEqual(evidence.read(self.out / 'status.json')['pi_comparison'], 'shared-comparison.json')

    def test_duplicate_or_missing_run_blocks_ci(self):
        _, result = self.fixture()
        result['results'][-1] = result['results'][0]
        evidence.dump(self.out / 'lumi/report.json', result)
        self.assertTrue(regression.report(self.out))

    def test_model_drift_and_environment_errors_block_ci(self):
        _, result = self.fixture()
        result['config']['model'] = 'changed'
        evidence.dump(self.out / 'lumi/report.json', result)
        self.assertTrue(regression.report(self.out))
        _, result = self.fixture()
        result['results'][0]['status'] = 'environment_error'
        evidence.dump(self.out / 'lumi/report.json', result)
        self.assertTrue(regression.report(self.out))
        result['results'][0]['status'] = 'agent_error'
        evidence.dump(self.out / 'lumi/report.json', result)
        self.assertFalse(regression.report(self.out))

    def test_missing_artifacts_produce_failure_record(self):
        self.assertTrue(regression.report(self.out))
        self.assertTrue((self.out / 'status.json').is_file())

    def test_retired_pi_execution_entrypoints_cannot_start(self):
        for action in ('init', 'prepare'):
            with patch('sys.argv', ['baseline.py', action]):
                with self.assertRaisesRegex(ValueError, 'retired'):
                    evidence.main()


if __name__ == '__main__':
    unittest.main()
