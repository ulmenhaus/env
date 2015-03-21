"""
Primitives for processing the input to and output from a function call
"""

from functools import wraps


class OptionlessDecorator(object):
    """
    A convenient super-class for function wrappers

    OptionlessDecorator provides the class method "decorate" which will
    properly wrap a function and initialize an instance of the
    OptionlessDecorator with the wrapped function. The decorator is called in
    place of the wrapped function and the decorator gets the name and
    docstring of the wrapped function.
    """

    def __init__(self, f):
        self.f = f

    @classmethod
    def decorate(cls, f):
        return wraps(f)(cls(f))
