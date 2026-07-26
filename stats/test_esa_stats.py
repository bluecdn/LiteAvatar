import importlib.util
import pathlib
import unittest


SCRIPT = pathlib.Path(__file__).with_name("esa-stats.py")
SPEC = importlib.util.spec_from_file_location("esa_stats", SCRIPT)
esa_stats = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(esa_stats)


class AvatarRequestMatcherTests(unittest.TestCase):
    def record(self, uri, host="gravatar.bluecdn.com", method="GET"):
        return {
            "ClientRequestHost": host,
            "ClientRequestMethod": method,
            "ClientRequestURI": uri,
        }

    def test_accepts_email_hash_qq_suffix_and_query(self):
        valid = [
            "/avatar/e1e7ba949ade0936e071132d2edd3b3c?s=80",
            "/avatar/" + "a" * 64,
            "/avatar/10000.png?size=64",
        ]
        for uri in valid:
            with self.subTest(uri=uri):
                self.assertTrue(esa_stats.is_valid_avatar_request(
                    self.record(uri), "gravatar.bluecdn.com"))

    def test_rejects_non_avatar_invalid_id_wrong_host_and_head(self):
        invalid = [
            self.record("/"),
            self.record("/favicon.ico"),
            self.record("/avatar/0"),
            self.record("/avatar/not-a-hash"),
            self.record("/avatar/" + "a" * 32, host="example.com"),
            self.record("/avatar/" + "a" * 32, method="HEAD"),
        ]
        for record in invalid:
            with self.subTest(record=record):
                self.assertFalse(esa_stats.is_valid_avatar_request(
                    record, "gravatar.bluecdn.com"))


if __name__ == "__main__":
    unittest.main()
