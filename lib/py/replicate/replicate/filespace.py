"""
tools for easy local persistence of replicable objects
"""

import functools
import json
import os
import shutil

from replicate.replicator import Replicator


pretty_dumps = functools.partial(json.dumps, indent=4)
standard_replicator = Replicator(serializer=pretty_dumps)


class Filespace(object):
    """
    provides a dict like interface for persisting files
    """
    # TODO a mechanism for listing the objects in a filespace (with globing)
    encoded_suffix = ".encoded"

    def __init__(self, root_dir, replicator=standard_replicator,
                 encode_strs=False):
        self.root_dir = root_dir
        self.replicator = replicator
        self.encode_strs = False

    def __getitem__(self, item):
        full_name = os.path.join(self.root_dir, item)
        is_encoded = False
        if os.path.exists(full_name + self.encoded_suffix):
            full_name = full_name + self.encoded_suffix
            is_encoded = True

        if not os.path.exists(full_name):
            os.mkdir(full_name)

        if os.path.isdir(full_name):
            return type(self)(full_name, self.replicator)

        process = self.replicator.deserialize if is_encoded else (lambda x: x)

        with open(full_name, 'r') as f:
            return process(f.read())

    def __setitem__(self, item, value):
        full_name = os.path.join(self.root_dir, item)

        # TODO should remove older string if present
        # (since it may have a different suffix)
        if not isinstance(value, str) or self.encode_strs:
            full_name = full_name + self.encoded_suffix
            value = self.replicator.serialize(value)

        # TODO should allow copying of entire filespaces
        with open(full_name, 'w') as f:
            f.write(value)

    def is_encoded_file(self, item):
        return item.endswith(self.encoded_suffix)

    def __truediv__(self, filename):
        return os.path.join(self.root_dir, filename)

    def __delitem__(self, item):
        full_path = os.path.join(self.root_dir, item)
        if not os.path.exists(full_path):
            raise ValueError(item)
        elif os.path.isdir(full_path):
            shutil.rmtree(full_path)
        else:
            os.remove(full_path)
