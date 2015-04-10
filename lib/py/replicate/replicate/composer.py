"""
decompose/recompose objects in the replication process
"""

from replicate.replicable import Replicable

from functionals.recursive import retire


class Composer(object):
    """
    decomposes and recomposes objects in the replication process

    A Composer has a decomposition_scheme which is a dict mapping types
    (or tuples of types) to actions which should be taken to decompose
    instances of those types. The action may be None in which case nothing
    is done, a callable which is called on the object, or an attribute
    name which is gotten from the object.
    """
    decomposition_scheme = {
        (int, float, str): None,
        (list, set, tuple): list,
        dict: None,
        Replicable: 'parts',
    }

    def __init__(self):
        self._action_cache = {}

    def get_action(self, o):
        """
        get the action for an object to be decomposed
        """
        if type(o) in self._action_cache:
            return self._action_cache[type(o)]

        for types, action in self.decomposition_scheme.items():
            if isinstance(o, types):
                self._action_cache[type(o)] = action
                return action

    def decompose(self, o):
        """
        decompose an object into its constituent parts
        """
        action = self.get_action(o)
        if action is None:
            retire(o)
        else:
            retire((yield (type(o), self._do_action(action, o))))

    def _do_action(self, action, o):
        if isinstance(action, str):
            return getattr(o, action)
        return action(o)

    def compose(self, pair):
        """
        recompose a decomposed object
        """
        if not isinstance(pair, tuple):
            retire(pair)

        o_type, o = pair
        unencoded = type(o)()

        if isinstance(o, dict):
            for key, part in o.items():
                unencoded[key] = (yield part)
            retire(o_type(**unencoded))
        else:
            for part in o:
                unencoded.append((yield part))
            retire(o_type(unencoded))
