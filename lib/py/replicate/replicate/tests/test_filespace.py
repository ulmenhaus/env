"""
unit tests for filespace module
"""

import os
import shutil
import tempfile
import unittest

from replicate import examples
from replicate.filespace import Filespace, standard_replicator


class FilespaceTestCase(unittest.TestCase):
    """
    Abstract base class for FileSpace test cases
    """
    def setUp(self):
        self.working_dir = tempfile.mkdtemp()
        self.filespace = Filespace(self.working_dir)

    def tearDown(self):
        shutil.rmtree(self.working_dir)

    def write_to(self, filename, contents=''):
        with open(os.path.join(self.working_dir, filename), 'w') as f:
            f.write(contents)

    def read_from(self, filename):
        with open(os.path.join(self.working_dir, filename), 'r') as f:
            return f.read()


class ReadToFilespace(FilespaceTestCase):
    def test_read_encoded_file(self):
        card = examples.Card(1, 'clubs')
        deck = examples.Deck([card], 'Acme')

        filename = "deck" + self.filespace.encoded_suffix
        self.write_to(filename, standard_replicator.serialize(deck))
        read_deck = self.filespace["deck"]

        self.assertEqual(deck, read_deck)

    def test_read_unencoded_file(self):
        contents = "ARMA VIRVMQVE CANO"
        self.write_to("Aeneid", contents)
        self.assertEqual(self.filespace["Aeneid"], contents)

    def test_read_directory(self):
        new_dir = os.path.join(self.working_dir, "new_dir")
        os.mkdir(new_dir)
        new_fs = self.filespace["new_dir"]
        self.assertEqual(type(self.filespace), type(new_fs))
        self.assertEqual(new_fs.root_dir, new_dir)


class WriteToFilespace(FilespaceTestCase):
    def test_write_encoded_file(self):
        card = examples.Card(1, 'clubs')
        deck = examples.Deck([card], 'Acme')

        filename = "written_deck" + self.filespace.encoded_suffix
        self.filespace["written_deck"] = deck
        contents = self.read_from(filename)

        self.assertEqual(contents, standard_replicator.serialize(deck))

    def test_write_unencoded_file(self):
        contents = "Nel mezzo del cammin di nostra vita"
        self.filespace["Inferno"] = contents
        self.assertEqual(self.read_from("Inferno"), contents)

    def test_make_directory(self):
        new_space = self.filespace["made_dir"]
        self.assertEqual(type(new_space), type(self.filespace))
        made_dir = os.path.join(self.working_dir, "made_dir")
        self.assertEqual(new_space.root_dir, made_dir)
        self.assertTrue(os.path.isdir(made_dir))


class DeleteInFilespace(FilespaceTestCase):
    def test_delete_file(self):
        self.write_to("foobar")
        full_path = os.path.join(self.working_dir, "foobar")
        self.assertTrue(os.path.exists(full_path))
        del self.filespace["foobar"]
        self.assertFalse(os.path.exists(full_path))

    def test_delete_directory(self):
        full_path = os.path.join(self.working_dir, "foodir")
        os.mkdir(full_path)
        self.assertTrue(os.path.exists(full_path))
        del self.filespace["foodir"]
        self.assertFalse(os.path.exists(full_path))
