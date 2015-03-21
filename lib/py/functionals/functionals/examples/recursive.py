"""
Example uses of the functionals.recursive module
"""

from functionals.recursive import CyclicRecursor, retire, Recursor


class MetaCircularEvaluator(CyclicRecursor):
    """
    A lisp-like meta-circular evaluator (an example of mutual recursion)

    May be used to evaluate a lisp-like expression where a lisp-like
    expression is one of the following:

    - A python atomic primitive
    - a tuple all of whose elements are lisp-like expressions and whose first
      element is callable when evaluated
    """
    def lisp_eval(expression):
        """
        If the expression denotes a call, make a recursive call to the apply
        function otherwise return the expression
        """
        if isinstance(expression, tuple):
            retire((yield expression))
        retire(expression)

    def lisp_apply(expression):
        """
        Make a recursive call to the eval function for each part in the
        expression and make the corresponding function call
        """
        evaluated = []
        for part in expression:
            evaluated.append((yield part))
        retire(evaluated[0](*evaluated[1:]))

    def __init__(self, eval_function=lisp_eval, apply_function=lisp_apply):
        super().__init__([eval_function, apply_function])

    def evaluate(self, expression):
        return self.recurse(expression)


@Recursor.decorate
def factorial(n):
    """
    A recursive definition of factorial with no explicit recursion depth limit
    """
    if n == 0:
        retire(1)
    retire(n * (yield n-1))


@Recursor.decorate
def fib(n):
    """
    An example of making multiple recursive calls
    """
    if n in [0, 1]:
        retire(n)
    retire((yield n-1) + (yield n-2))
