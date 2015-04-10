"""
example use of the replicate package
"""

from replicate.replicable import Replicable, preprocessor


class Card(Replicable):
    """
    a model of a playing card
    """
    @preprocessor
    def preprocess(rank, suit):
        pass


class Deck(Replicable):
    """
    a model of a deck of playing cards
    """
    @preprocessor
    def preprocess(cards, brand):
        pass
