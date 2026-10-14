"""Text normalization shared by g2p.py and align.py.

Real-world sentences carry two things neither espeak nor uroman handle
cleanly when they're glued onto a word: surrounding punctuation (quote
marks, Persian/Arabic commas and question marks, etc.) and digit-written
numbers.

Punctuation: espeak-ng's phonemizer doesn't just ignore an attached
punctuation mark, it can corrupt the whole word's output around it (a
stray guillemet before "اقدامات" made espeak emit a spurious digit-like
phoneme in the middle of the word, observed directly in testing) --
stripping it before phonemizing/romanizing avoids that outright, rather
than trying to filter whatever espeak does with it afterward.

Numbers: espeak's Farsi/Hebrew phonemizer already reads digits as
spoken-out numbers on its own (e.g. "28" phonemizes the same as "بیست و
هشت" would) -- but uroman does not, it transliterates digits literally
("28" -> "28"), and the forced aligner's vocabulary is just a-z plus a
few punctuation marks, no digits. So alignment needs the number spelled
out before romanizing it, even though G2P doesn't.
"""

import re
import unicodedata

from num2words import num2words

_LEADING_TRAILING_PUNCT_RE = re.compile(r"^[^\w]+|[^\w]+$", re.UNICODE)

_NUM2WORDS_LANG = {"farsi": "fa", "hebrew": "he"}


def strip_punctuation(word: str) -> str:
    """Drop leading/trailing punctuation (quote marks, Persian/Arabic
    commas and question marks, etc.) -- keeps internal characters as-is,
    so e.g. an internal hyphen is untouched."""
    return _LEADING_TRAILING_PUNCT_RE.sub("", word)


def spell_out_if_numeral(word: str, language: str) -> str:
    """If `word` is entirely digits (Persian, Arabic-Indic, or ASCII),
    return it spelled out as words, joined with no spaces so it still
    counts as a single alignment unit. Returns `word` unchanged if it
    isn't a plain integer (e.g. contains a decimal point) -- the caller
    will then see uroman/the aligner's own error for it, same as before
    this normalization existed."""
    if not word or not word.isdigit():
        return word
    lang_code = _NUM2WORDS_LANG.get(language)
    if not lang_code:
        return word
    try:
        n = int(unicodedata.normalize("NFKC", word))
    except ValueError:
        return word
    return num2words(n, lang=lang_code).replace(" ", "")
