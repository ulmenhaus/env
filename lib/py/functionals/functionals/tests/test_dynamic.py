from unittest import TestCase

from functionals.examples.dynamic import fib


class MemoizerTestCase(TestCase):
    pass


class BasicMemoization(MemoizerTestCase):
    def test_fib(self):
        self.assertEqual(354224848179261915075, fib(100))
