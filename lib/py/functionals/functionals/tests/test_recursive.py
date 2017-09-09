import hashlib
import operator

from unittest import TestCase

from functionals.examples.recursive import (MetaCircularEvaluator,
                                            factorial, fib)


class MCETestCase(TestCase):
    evaluator_class = MetaCircularEvaluator

    def setUp(self):
        self.evaluator = self.evaluator_class()

    def assertEvaluatesTo(self, expression, value):
        self.assertEqual(self.evaluator.evaluate(expression), value)


class BasicEvaluation(MCETestCase):
    def test_no_application(self):
        self.assertEvaluatesTo(12, 12)

    def test_single_application(self):
        expression = (operator.add, 5, 3)
        self.assertEvaluatesTo(expression, 8)

    def test_single_level_recursion(self):
        join = lambda *strs: " ".join(strs)

        expression = (join,
                      (join, "foo", "bar"),
                      (join, "garpley", "baz"))
        self.assertEvaluatesTo(expression, "foo bar garpley baz")

    def test_multi_level_recursion(self):
        expression = (operator.add,
                      (operator.mul, (operator.add, 5, 5), 20),
                      (operator.mul, 10, 30))
        self.assertEvaluatesTo(expression, 500)


class RecursorTestCase(TestCase):
    pass


class BasicRecursing(RecursorTestCase):
    def test_no_recursion_limit_edge(self):
        encoded_factorial = str(factorial(2000)).encode()
        self.assertEqual(hashlib.md5(encoded_factorial).hexdigest(),
                         '13e8fc22631f0d2d6a44c9d72704eb6f')

    def test_no_recursion_limit_major(self):
        encoded_factorial = str(factorial(10000)).encode()
        self.assertEqual(hashlib.md5(encoded_factorial).hexdigest(),
                         '19b7ef180d483270f3acb82f431acd44')

    def test_parallel_recursive_calls(self):
        self.assertEqual([fib(n) for n in range(10)],
                         [0, 1, 1, 2, 3, 5, 8, 13, 21, 34])
