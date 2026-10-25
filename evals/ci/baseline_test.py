"""Offline evidence-validation tests. Synthetic pi records are never run evidence."""
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import baseline as b


class BaselineTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.out = Path(self.temp.name)
        self.cfg = b.config({'EVAL_MODEL': 'unit-test-model', 'EVAL_BASE_URL': 'https://example.invalid/v1',
                             'EVAL_REPETITIONS': '2'})

    def pi_fixture(self, score=0):
        root = self.out / 'pi' / 'unit-test-only'
        tasks = [dict(evalSet=scenario[0], caseId=scenario[1], variant=variant,
                      model='lumi-eval/' + self.cfg['model'], runNumber=repeat)
                 for scenario in b.SCENARIOS for variant in ('with_docs', 'without_docs') for repeat in (1, 2)]
        protocol = {'schemaVersion': 1, 'runsPerVariant': self.cfg['repetitions'],
                    'model': 'lumi-eval/' + self.cfg['model'], 'tasks': tasks}
        digest = b.sha(json.dumps(protocol, separators=(',', ':')).encode())
        b.dump(root / 'protocol.json', dict(protocol, protocolDigest=digest))
        b.dump(root / 'expected-runs.json', tasks)
        b.dump(root / 'report.json', {'schemaVersion': 3, 'protocolDigest': digest, 'blockedPairs': []})
        observations = [dict(t, outcome='scored', score=score, totalMs=10) for t in tasks]
        (root / 'observations.jsonl').write_text('\n'.join(json.dumps(t) for t in observations) + '\n')
        for task in tasks:
            key = b.sha(json.dumps(list(b.pi_identity(task)), separators=(',', ':')).encode())
            b.dump(root / 'tasks' / key / 'vitest.json', {'unit_test_fixture': True})
            session = root / task['variant'] / 'sessions' / key / 'session.jsonl'
            session.parent.mkdir(parents=True, exist_ok=True)
            session.write_text('{"unit_test_fixture":true}\n')
        return root, observations

    def test_low_scores_are_valid_and_unavailable_metrics_stay_null(self):
        self.pi_fixture()
        rows, blocked, _ = b.load_pi(self.out / 'pi', self.cfg)
        self.assertFalse(blocked)
        stats = b.summarize(next(iter(rows.values())), 2)
        self.assertEqual(stats['pass_rate'], 0)
        self.assertEqual(stats['scored'], 2)
        self.assertIsNone(stats['input_tokens']['total'])
        self.assertEqual(stats['input_tokens']['available_runs'], 0)

    def test_duplicate_observation_rejected(self):
        root, observations = self.pi_fixture()
        with (root / 'observations.jsonl').open('a') as stream:
            stream.write(json.dumps(observations[0]) + '\n')
        with self.assertRaisesRegex(ValueError, 'duplicates'):
            b.load_pi(self.out / 'pi', self.cfg)

    def test_missing_run_is_not_counted_as_zero(self):
        root, observations = self.pi_fixture(score=1)
        (root / 'observations.jsonl').write_text('\n'.join(json.dumps(t) for t in observations[1:]))
        rows, blocked, _ = b.load_pi(self.out / 'pi', self.cfg)
        self.assertTrue(any('missing 1' in s for s in blocked))
        first = b.summarize(next(iter(rows.values())), 2)
        self.assertEqual(first['pass_rate'], 1)
        self.assertEqual(first['scored'], 1)
        self.assertEqual(first['planned'], 2)

    def test_error_has_unknown_attribution_not_agent_zero(self):
        root, observations = self.pi_fixture(score=1)
        observations[0]['outcome'] = 'errored'
        del observations[0]['score']
        (root / 'observations.jsonl').write_text('\n'.join(json.dumps(t) for t in observations))
        rows, blocked, _ = b.load_pi(self.out / 'pi', self.cfg)
        self.assertTrue(any('attribution unavailable' in s for s in blocked))
        first = b.summarize(next(iter(rows.values())), 2)
        self.assertEqual(first['scored'], 1)
        self.assertEqual(first['pass_rate'], 1)
        self.assertEqual(first['statuses']['errored'], 1)

    def test_changed_model_and_protocol_rejected(self):
        root, _ = self.pi_fixture()
        wrong = dict(self.cfg, model='other-model')
        with self.assertRaisesRegex(ValueError, 'model mismatch'):
            b.load_pi(self.out / 'pi', wrong)
        protocol = b.read(root / 'protocol.json')
        protocol['model'] = 'modified'
        b.dump(root / 'protocol.json', protocol)
        with self.assertRaisesRegex(ValueError, 'digest mismatch'):
            b.load_pi(self.out / 'pi', self.cfg)

    def test_existing_real_lumi_record_is_read_only_and_maps_all_six_cases(self):
        root = b.HERE.parent / 'records/2026-10-25-pi-reference-qwen3.7-plus'
        original = b.read(root / 'report.json')
        cfg = dict(original['config'], timeout_seconds=original['config']['timeout_seconds_per_case'],
                   lumi_max_requests_per_turn=original['config']['max_requests_per_turn'],
                   lumi_provider_type=original['config']['provider_type'])
        before = b.sha((root / 'report.json').read_bytes())
        rows, blocked = b.load_lumi(root, cfg)
        self.assertFalse(blocked)
        self.assertEqual(len(rows), 6)
        self.assertEqual(sum(len(r) for r in rows.values()), 18)
        self.assertEqual(sum(b.summarize(r, 3)['successes'] for r in rows.values()), 8)
        self.assertEqual(before, b.sha((root / 'report.json').read_bytes()))

    def test_missing_evidence_still_produces_reviewable_report(self):
        with patch.dict('os.environ', {'GITHUB_STEP_SUMMARY': ''}):
            report = b.report(self.out)
        self.assertTrue(report['blocked'])
        self.assertEqual(len(report['rows']), 6)
        self.assertTrue((self.out / 'comparison.md').exists())
        self.assertTrue(all(r['delta_percentage_points'] is None for r in report['rows']))

    def test_secret_and_symlink_rejected_before_upload(self):
        (self.out / 'trace.json').write_text('secret-key-unit-test')
        with self.assertRaisesRegex(ValueError, 'Credential'):
            b.seal(self.out, 'secret-key-unit-test')
        (self.out / 'trace.json').write_text('safe')
        b.seal(self.out, 'secret-key-unit-test')
        self.assertEqual(b.read(self.out / 'sha256-manifest.json')['trace.json'], b.sha(b'safe'))
        (self.out / 'link').symlink_to('/tmp')
        with self.assertRaisesRegex(ValueError, 'symlinks'):
            b.seal(self.out, 'secret-key-unit-test')

    def test_config_limits_and_credentials_in_url(self):
        for endpoint in ('http://example.invalid/v1', 'https://key:secret@example.invalid',
                         'https://example.invalid/v1?api_key=secret'):
            with self.assertRaises(ValueError):
                b.config({'EVAL_BASE_URL': endpoint, 'EVAL_MODEL': 'model'})
        with self.assertRaises(ValueError):
            b.config({'EVAL_BASE_URL': 'https://example.invalid', 'EVAL_MODEL': 'model', 'EVAL_REPETITIONS': '11'})

    def test_variation_and_invalid_metrics(self):
        stats = b.summarize([{'score': 0, 'status': 'scored'}, {'score': 1, 'status': 'scored'}], 2)
        self.assertEqual(stats['score']['stddev'], 0.5)
        for bad in (float('nan'), float('inf'), -1, True):
            with self.assertRaises(ValueError):
                b.metrics([{'duration_ms': bad}], 'duration_ms')


if __name__ == '__main__':
    unittest.main()
