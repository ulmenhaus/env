"""
Models a drawing in python that can be converted to pic

This allows you to use all Python language features like
composability and comprehensions to express drawings more
richly
"""

import contextlib


class Instruction(object):
    pass


class RawInstruction(Instruction):
    def __init__(self, ixn):
        self.ixn = ixn

    def __repr__(self):
        return self.ixn


class AssignmentInstruction(Instruction):
    def __init__(self, var, val):
        self.var = var
        self.val = val

    def __repr__(self):
        return "{} = {}".format(self.var, self.val)


class NameInstruction(Instruction):
    def __init__(self, var, val):
        self.var = var
        self.val = val

    def __repr__(self):
        return "{}: {}".format(self.var, self.val)


class Term(object):
    def __init__(self, s, drawing=None):
        self._s = s
        self.drawing = drawing

    def __add__(self, other):
        return Term("({} + {})".format(self, other))

    def __sub__(self, other):
        return Term("({} - {})".format(self, other))

    def __mul__(self, other):
        return Term("({} * {})".format(self, other))

    def __truediv__(self, other):
        return Term("({} / {})".format(self, other))

    def __getattr__(self, other):
        return Term("{}.{}".format(self, other))

    def __call__(self, *args, **kwargs):
        elem = ElemInstruction(self._s, *args, **kwargs)
        self.drawing.add(elem)
        return elem

    def __repr__(self):
        return self._s


class ElemInstruction(object):
    def __init__(self, *args, **kwargs):
        self.args = args
        cleaned = {}
        for k, v in kwargs.items():
            if k.endswith("_"):
                cleaned[k[:-1]] = v
            else:
                cleaned[k] = v
        self.kwargs = cleaned
        self.with_args = {}
        self.thens = []
        self.post = ""

    def __repr__(self):
        rendered = " ".join(map(render, self.args))
        if self.kwargs:
            rendered += " " + " ".join("{} {}".format(k, v)
                                       for k, v in self.kwargs.items())
        if self.with_args:
            rendered += " with " + " ".join(".{} at {}".format(k, v)
                                            for k, v in self.with_args.items())

        for d in self.thens:
            rendered += " then " + " ".join("{} {}".format(k, v)
                                            for k, v in d.items())

        return rendered + self.post

    def with_(self, **kwargs):
        self.with_args = kwargs
        return self

    def then(self, **kwargs):
        self.thens.append(kwargs)
        return self

    def __call__(self, s):
        if self.post:
            self.post += ' {}'.format(s)
        else:
            self.post += ' "{}"'.format(s)
        return self


class Drawing(object):
    def __init__(self):
        self.ixns = []

    def render(self):
        ixns = [RawInstruction(".PS")] + self.ixns + [RawInstruction(".PE\n")]
        return "\n".join(map(render, ixns))

    def add(self, *ixns):
        self.ixns.extend(ixns)

    def __call__(self, raw_ixn):
        self.ixns.append(RawInstruction(raw_ixn))

    def __getattr__(self, attr):
        if attr in ["add", "render", "ixns"]:
            return self.__dict__[attr]
        return Term(attr, drawing=self)

    def __getitem__(self, item):
        return getattr(self, item)

    def __setitem__(self, k, v):
        self.ixns.append(AssignmentInstruction(k, v))

    def __setattr__(self, attr, v):
        if attr in ["add", "render", "ixns"]:
            self.__dict__[attr] = v
            return
        if v is not self.ixns[-1]:
            raise ValueError("Doing an assignment on not the last element")
        self.ixns[-1] = NameInstruction(attr, v)

    @contextlib.contextmanager
    def nested(self):
        self.ixns.append(RawInstruction("{"))
        yield
        self.ixns.append(RawInstruction("}"))


def render(obj):
    if isinstance(obj, Instruction):
        return repr(obj)
    return str(obj)
