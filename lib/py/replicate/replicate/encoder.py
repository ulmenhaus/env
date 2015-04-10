"""
tools to encode replicable objects as python primitives
"""


import importlib

from functionals.recursive import retire


class Encoder(object):
    """
    encodes replicable objects as python primitives
    """
    constructors_by_typeid = {}
    typeids_by_constructor = {}

    def encode(self, pair):
        """
        encode an object as a python primitive

        Input may be a:
        - primitive in which case nothing is done to it
        - (type, parts) pair in which case the parts are recursively
          decomposed/encoded and a dict is returned
        """
        if not isinstance(pair, tuple):
            retire(pair)

        o_type, o = pair
        decomposed = type(o)()

        if hasattr(o, 'items'):
            for key, part in o.items():
                decomposed[key] = (yield part)
        else:
            for part in o:
                decomposed.append((yield part))

        retire({
            'type': self.encode_type(o_type),
            'parts': decomposed,
        })

    @classmethod
    def register_constructor(cls, type_to_register, type_name):
        """
        register a constructer with a global identifier
        """
        if type_name in cls.constructors_by_typeid:
            raise KeyError(type_name)
        cls.constructors_by_typeid[type_name] = type_to_register
        cls.typeids_by_constructor[type_name] = type_name

    @staticmethod
    def default_identifier(cls):
        """
        get the default identifier for a constructor
        """
        return ".".join([cls.__module__, cls.__name__])

    @staticmethod
    def decode_default_identifier(encoded_type):
        """
        default function to get the constructor for a global identifier
        """
        parts = encoded_type.split(".")
        module_name, item_name = ".".join(parts[:-1]), parts[-1]
        return getattr(importlib.import_module(module_name), item_name)

    def encode_type(self, o_type):
        """
        return the global identifier for a constructor
        """
        default = self.default_identifier(o_type)
        return self.typeids_by_constructor.get(o_type, default)

    def decode_type(self, encoded_type):
        """
        return the constructor for a global identifier
        """
        if encoded_type in self.constructors_by_typeid:
            return self.constructors_by_typeid[encoded_type]
        return self.decode_default_identifier(encoded_type)

    def decode(self, encoded_o):
        """
        decode a dict-encoded object
        """
        if not isinstance(encoded_o, dict):
            retire(encoded_o)
        decoded = (self.decode_type(encoded_o['type']),
                   encoded_o['parts'])
        retire((yield decoded))
