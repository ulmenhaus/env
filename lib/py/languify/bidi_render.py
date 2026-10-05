"""Terminal RTL rendering helpers.

ncurses has no concept of bidi reordering -- it just draws each string
left-to-right, one cell per character, in exactly the order it is given.
Kitty (run under tmux, this tool's target setup) compensates for that
itself, but *differently* for the two scripts this tool handles:

- Hebrew: Kitty renders each word's own letters correctly, but does
  nothing about word order or attached punctuation -- both need to be
  handled here (reverse the order of words, and move punctuation
  attached to a word's start/end to the opposite end, mirroring
  characters that have a directional counterpart like parens or "?" ->
  "؟"). Confirmed via a side-by-side on-screen comparison
  (languify_rtl_diag.py) against real Hebrew text.

- Farsi (Arabic script): Kitty's Arabic-script support is more complete
  (Arabic has far broader terminal/font engineering behind it than
  Hebrew), and it turns out to *already* reposition punctuation attached
  to a word on its own -- so flipping it here too double-processes it
  and lands back on the wrong side. Only word order needs reversing here;
  attached punctuation is left exactly as typed. Also confirmed via a
  side-by-side comparison (languify_farsi_diag2.py) against real text
  from hp_farsi/11.txt.

Farsi letters are still reshaped into their joined presentation forms
first (a word-local operation Kitty does not do itself; Hebrew has no
positional letter forms, so it skips this step).

Line-wrapping must happen on the *logical* string first (see
interactive.py) -- reordering, then wrapping, would break words apart in
the wrong places.
"""

_MIRROR_MAP = {
    "(": ")", ")": "(",
    "[": "]", "]": "[",
    "{": "}", "}": "{",
    "<": ">", ">": "<",
    "“": "”", "”": "“",  # “ ”
    "‘": "’", "’": "‘",  # ‘ ’
    "«": "»", "»": "«",  # « »
    "?": "؟",  # Arabic/Farsi mirrored question mark: ؟
}


def _mirror_char(ch: str) -> str:
    return _MIRROR_MAP.get(ch, ch)


def _flip_attached_punctuation(token: str) -> str:
    """Move punctuation attached to the start/end of `token` (a trailing
    comma/period/question mark, a leading open-paren/quote, ...) to the
    opposite end, mirroring characters that have a directional
    counterpart. A token with no attached punctuation is returned as-is.

    Hebrew only -- Kitty already does this itself for Arabic-script
    (Farsi) text; see module docstring.
    """
    start, end = 0, len(token)
    while start < end and not token[start].isalnum():
        start += 1
    while end > start and not token[end - 1].isalnum():
        end -= 1

    leading, core, trailing = token[:start], token[start:end], token[end:]
    if not core or (not leading and not trailing):
        # `not core`: the token has no attached word at all -- e.g. a
        # standalone dash, quote mark, or stray OCR artifact that just
        # happens to have no space around it. There's no "other side of
        # the word" to move such a token to, so mirroring it in place
        # (a lone "<" becoming a lone ">") is simply wrong; leave it.
        return token

    new_leading = "".join(_mirror_char(c) for c in reversed(trailing))
    new_trailing = "".join(_mirror_char(c) for c in reversed(leading))
    return new_leading + core + new_trailing


def to_visual(line: str, needs_reshaping: bool) -> str:
    if needs_reshaping:
        try:
            import arabic_reshaper
            line = arabic_reshaper.reshape(line)
        except ImportError:  # degrades to unshaped Farsi glyphs
            pass
        tokens = line.split(" ")
    else:
        tokens = [_flip_attached_punctuation(tok) for tok in line.split(" ")]

    return " ".join(reversed(tokens))
