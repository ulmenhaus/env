"""
tools to replicate objects
"""


import json

from functionals.recursive import CyclicRecursor

from replicate.composer import Composer
from replicate.encoder import Encoder


class Replicator(object):
    """
    object replicator
    """
    def __init__(self, composer=Composer(), encoder=Encoder(),
                 serializer=json.dumps, deserializer=json.loads):
        self.serializer = CyclicRecursor([composer.decompose, encoder.encode],
                                         postprocessor=serializer)
        self.deserializer = CyclicRecursor([encoder.decode, composer.compose],
                                           preprocessor=deserializer)

    def serialize(self, o):
        """
        serialize a replicable object
        """
        return self.serializer.recurse(o)

    def deserialize(self, encoded_o):
        """
        deserialize a replicable object
        """
        return self.deserializer.recurse(encoded_o)

    def replicate(self, o):
        """
        replicate an object
        """
        return self.deserialize(self.serialize(o))
