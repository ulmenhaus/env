"""
tools for defining replicable objects
"""

import inspect

from replicate.encoder import Encoder


class GloballyIdentifiedClass(type):
    """
    metaclass for classes that need a global identifier

    A globally identified class is a constructor that can be globally
    identified by a name. Any class definition that is an instance of this
    meta-class will automatically registered with the Encoder class as a
    globally identified class.
    """
    registry = Encoder

    def __init__(cls, name, bases, nmspc):
        super().__init__(name, bases, nmspc)
        cls.registry.register_constructor(cls, cls.cls_identifier)

    @property
    def cls_identifier(cls):
        """
        The global identifier for the class
        """
        if hasattr(cls, 'get_cls_identifier'):
            return cls.get_cls_identifier()
        return Encoder.default_identifier(cls)


def preprocessor(f):
    """
    decorator denoting a preprocessor method (see Replicable)
    """
    f._is_preprocessor = True
    return staticmethod(f)


def primary_preprocessor(f):
    """
    decorator denoting the primary preprocessor method (see Replicable)
    """
    f = preprocessor(f)
    f._is_primary = True
    return f


class Replicable(object, metaclass=GloballyIdentifiedClass):
    """
    Base class for replicable objects

    Any subclass should define one or more preprocessors. A preprocessor is a
    static method decorated with replicable.preprocessor. When initializing,
    the Replicable will find a preprocessor matching the instantiation
    positional/keyword-arguments, take the returned dict from the
    preprocessor, and update its own attributes from the entries in that dict.
    If None is returned, a dict is made from the call context of the function.
    """
    def __init__(self, *args, **kwargs):
        processors = [self.primary_preprocessor] + list(self.preprocessors)
        selected_preprocessor = None
        for preprocessor in processors:
            try:
                context = inspect.getcallargs(preprocessor, *args, **kwargs)
                selected_preprocessor = preprocessor
            except Exception:
                pass

        if not selected_preprocessor:
            raise TypeError("Invalid instantiation arguments", args, kwargs)

        processed_attrs = selected_preprocessor(*args, **kwargs)
        if processed_attrs is None:
            processed_attrs = context
        for attr_name, attr in processed_attrs.items():
            setattr(self, attr_name, attr)

    @property
    def preprocessors(self):
        """
        yield all preprocessors for a replicable
        """
        for _name, attr in inspect.getmembers(type(self)):
            if getattr(attr, '_is_preprocessor', False):
                yield attr

    @property
    def primary_preprocessor(self):
        """
        return the primary preprocessor for the replicable
        """
        preprocessor = None
        for preprocessor in self.preprocessors:
            if getattr(preprocessor, '_is_primary', False):
                return preprocessor
        return preprocessor

    @property
    def parts(self):
        """
        the parts of the replicable which must be replicated

        by default these are the parameters to the replicable's primary
        preprocessor (or any preprocessor if there is no primary one)
        """
        if hasattr(self, '_parts'):
            return self._parts
        argspec = inspect.getargspec(self.primary_preprocessor)
        needed_attrs = {attr_name: getattr(self, attr_name)
                        for attr_name in argspec.args}
        if argspec.varargs:
            needed_attrs[argspec.varargs] = getattr(self, argspec.varargs)

        if argspec.keywords:
            needed_attrs[argspec.keywords] = getattr(self, argspec.keywords)

        return needed_attrs

    def __eq__(self, other):
        return type(self) == type(other) and self.parts == other.parts

    def __hash__(self):
        return hash(frozenset(self.parts.items()))

    @parts.setter
    def parts(self, parts):
        self._parts = parts
