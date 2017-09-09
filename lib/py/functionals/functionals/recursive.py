"""
Tools for writing recursive-style functions that don't push onto the call-stack

The documentation for this module will repeatedly use the phrase,
"pseudo-recursive function." A pseudo-recursive function is a generator which
yields requests for recursive calls. By translating a recursive function into
a pseudo-recursive one a programmer may run the function without an explicit
recursion depth limit using the tools in this module.
"""

from contextlib import contextmanager

from functionals.wrappers import OptionlessDecorator


class CallRequest(object):
    """
    A canonical representation of a function's request for a recursive call
    """
    def __init__(self, *args, **kwargs):
        self.args = args
        self.kwargs = kwargs


def recurse(*args, **kwargs):
    """
    Create a request for a recursive call
    """
    return CallRequest(*args, **kwargs)


def retire(value):
    """
    The equivalent of "return" for a pseudo-recursive definition
    """
    raise StopIteration(value)


class StopRecursion(StopIteration):
    """
    Raised when the original function call has retired
    """
    pass


class RecursiveCaller(object):
    """
    Manages a single call to a pseudo-recursive function
    """
    def __init__(self, recursor, input_args, input_kwargs):
        self.recursor = recursor
        self.call_requests = []
        self.return_requests = []
        self.input_args = input_args
        self.input_kwargs = input_kwargs
        self.returns_to = {}
        self.generator_of = {}

    def call_and_log(self, generator, args, kwargs):
        iterator = generator(*args, **kwargs)
        self.generator_of[iterator] = generator
        return iterator

    def recurse(self):
        generator = self.recursor.get_successor()
        iterator = self.call_and_log(generator,
                                     self.input_args,
                                     self.input_kwargs)

        self.returns_to[iterator] = None
        self.generator_of[iterator] = generator

        self.append_next_request(iterator)

        while True:
            try:
                self._do_call_requests()
                self._do_return_requests()
            except StopRecursion as s:
                return s.value

    def _do_call_requests(self):
        while self.call_requests:
            iterator, req = self.call_requests.pop(0)
            req = self._canonicalize_request(req)
            generator = self.generator_of[iterator]
            next_generator = self.recursor.get_successor(generator)
            next_iterator = next_generator(*req.args, **req.kwargs)
            self.generator_of[next_iterator] = next_generator
            self.returns_to[next_iterator] = iterator
            self.append_next_request(next_iterator)

    def _do_return_requests(self):
        while self.return_requests:
            iterator, value = self.return_requests.pop(0)
            if iterator is None:
                raise StopRecursion(value)
            self.send_and_append_next_request(iterator, value)

    @contextmanager
    def check_for_retires(self, iterator):
        try:
            yield
        except StopIteration as s:
            self.return_requests.append((self.returns_to[iterator], s.value))
            del self.returns_to[iterator]
            del self.generator_of[iterator]

    def append_next_request(self, iterator):
        with self.check_for_retires(iterator):
            self.call_requests.append((iterator, next(iterator)))

    def send_and_append_next_request(self, iterator, value):
        with self.check_for_retires(iterator):
            next_request = iterator.send(value)
            self.call_requests.append((iterator, next_request))

    def _canonicalize_request(self, request):
        if isinstance(request, CallRequest):
            return request
        return CallRequest(request)


class CyclicRecursor(object):
    """
    An evaluator for cyclic pseudo-recursive functions

    Some computational tasks are conveniently defined using two functions
    which make calls to each other (consider the eval and apply functions of a
    meta-circular evaluator). The more general case of this phenomenon is one
    where n functions form a cycle where each function makes recursive calls
    to its successor in the cycle. Note that standard recursion provides an
    example of this phenomenon where n=1 and mutual recursion provides an
    example where n=2.

    A CyclicRecursor may be initialized with a list pseudo-recursive functions
    representing the successor graph of cyclically recursive functions. Call
    requests yielded from the first generator will result in calls to the
    second generator whose results are sent back to the first generator. Call
    requests from the second generator will result in calls to the third
    generator and so on.
    """
    pack = lambda *args, **kwargs: (args, kwargs)
    identity = lambda x: x

    def __init__(self, generators, preprocessor=pack, postprocessor=identity):
        self.generators = generators
        self.preprocessor = preprocessor
        self.postprocessor = postprocessor
        self.successors = {
            generators[i]: generators[i+1]
            for i in range(len(generators) - 1)
        }
        self.successors[generators[-1]] = generators[0]

    def recurse(self, *args, **kwargs):
        args, kwargs = self.preprocess(args, kwargs)
        recursive_caller = RecursiveCaller(self, args, kwargs)
        return self.postprocessor(recursive_caller.recurse())

    def preprocess(self, args, kwargs):
        result = self.preprocessor(*args, **kwargs)
        if isinstance(result, tuple):
            if len(result) == 2 and isinstance(result[1], dict):
                return result
            return result, {}
        return (result,), {}

    def get_successor(self, generator=None):
        if generator is None:
            return self.generators[0]
        return self.successors.get(generator)


class Recursor(CyclicRecursor, OptionlessDecorator):
    """
    A decorator denoting a pseudo-recursive function
    """
    def __init__(self, f):
        CyclicRecursor.__init__(self, [f])
        OptionlessDecorator.__init__(self, f)

    def __call__(self, *args, **kwargs):
        return self.recurse(*args, **kwargs)
