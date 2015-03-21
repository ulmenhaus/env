"""
Tools for dynamic programming
"""

from functionals.wrappers import OptionlessDecorator


class Memoizer(OptionlessDecorator):
    """
    A decorator for memoizing functions
    """
    def __init__(self, f):
        super().__init__(f)
        self.previous = {}

    def __call__(self, *args, **kwargs):
        key = args, frozenset(kwargs.items())
        if key in self.previous:
            return self.previous[key]
        result = self.f(*args, **kwargs)
        self.previous[key] = result
        return result
