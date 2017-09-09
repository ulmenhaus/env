"""
unit tests for the examples module
"""

import unittest

from replicate import examples
from replicate.replicator import Replicator


class ReplicatorTestCase(unittest.TestCase):
    """
    Abstract base class for Replicator tests
    """
    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self.replicator = Replicator()


class BasicInterfacing(ReplicatorTestCase):
    """
    test basic replication of objects
    """
    def test_replicate_primitive(self):
        """
        test replication of example Card object
        """
        card = examples.Card(1, 'clubs')
        self.assertEqual(card.rank, 1)
        self.assertEqual(card.suit, 'clubs')
        card_copy = self.replicator.replicate(card)

        self.assertNotEqual(id(card), id(card_copy))
        self.assertEqual(card, card_copy)

        self.assertEqual(card.rank, card_copy.rank)
        self.assertEqual(card.suit, card_copy.suit)

    def test_replicate_composite(self):
        """
        test replication of example Deck object
        """
        card = examples.Card(1, 'clubs')
        deck = examples.Deck([card], 'Acme')
        self.assertEqual(deck.cards, [card])
        self.assertEqual(deck.brand, 'Acme')

        deck_copy = self.replicator.replicate(deck)

        self.assertNotEqual(id(deck_copy), id(deck))
        self.assertEqual(deck_copy, deck)

        self.assertNotEqual(id(deck_copy.cards), id(deck.cards))
        self.assertEqual(deck_copy.cards, deck.cards)
        self.assertEqual(deck_copy.brand, deck.brand)

        self.assertNotEqual(id(deck_copy.cards[0]), id(deck.cards[0]))
