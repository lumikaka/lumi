"""Counter and leakage checks for the acceptance reporter, not simulated host behavior."""
import unittest

from storyboard_audit import content_audit, usage


def event(input_tokens, cached, output, last):
    return {'type': 'event_msg', 'payload': {'type': 'token_count', 'info': {
        'total_token_usage': {'input_tokens': input_tokens, 'cached_input_tokens': cached,
                              'output_tokens': output}, 'last_token_usage': {'input_tokens': last}}}}


class UsageTest(unittest.TestCase):
    def test_repeated_cumulative_updates_are_counted_once_and_peak_uses_last(self):
        result = usage([event(100, 20, 10, 100), event(100, 20, 10, 100), event(350, 200, 35, 250)])
        self.assertEqual(result['input_tokens'], 350)
        self.assertEqual(result['uncached_input_tokens'], 150)
        self.assertEqual(result['output_tokens'], 35)
        self.assertEqual(result['single_input_peak'], 250)

    def test_missing_usage_is_not_zero_and_reset_is_not_silently_ignored(self):
        for events in ([], [event(100, 120, 5, 100)], [event(100, 0, 5, 100), event(50, 0, 5, 50)]):
            with self.assertRaises(ValueError):
                usage(events)

    def test_content_checks_decode_envelopes_and_cover_multiple_tool_outputs(self):
        novel = [('ch1', 'title', 'first paragraph\nsecond paragraph')]
        pages = [{'chapter_uuid': 'ch1', 'pages': [{'current_storyboard': {'content_md': 'page one'}},
                                                {'current_storyboard': {'content_md': 'page two'}}]}]
        result = content_audit([{'output': '{"text":"first paragraph"}'}, {'output': 'second paragraph'}], novel, pages)
        self.assertFalse(result['passed'])
        result = content_audit([{'output': 'page one'}, {'output': 'page two'}], novel, pages)
        self.assertFalse(result['passed'])
        result = content_audit([{'output': 'chapter summary and next handoff'}], novel, pages)
        self.assertTrue(result['passed'])
        with self.assertRaises(ValueError):
            content_audit([], novel, [{'chapter_uuid': 'ch1', 'pages': [{'has_storyboard': True}]}])


if __name__ == '__main__':
    unittest.main()
