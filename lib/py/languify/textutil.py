"""Sentence splitting for the interactive reader.

This is a practical heuristic, not a real tokenizer: it collapses each
paragraph's internal whitespace, then cuts on `.`, `!`, `?`, or the
Farsi/Arabic question mark `؟` followed by whitespace. That is enough to
give the --interactive UI reasonably-sized, reasonably-natural chunks to
bold and read aloud one at a time; it does not need to be exact, since
each chunk is still played back as one continuous TTS utterance.

(Combine-mode TTS, by contrast, hands each engine the whole text at once
and lets Piper/israwave do their own internal sentence segmentation.)
"""

import re
from typing import List

_PARAGRAPH_RE = re.compile(r"\n\s*\n+")
_SENTENCE_BOUNDARY_RE = re.compile(r"(?<=[.!?؟])\s+")


def split_sentences(text: str) -> List[str]:
    sentences: List[str] = []
    for para in _PARAGRAPH_RE.split(text):
        collapsed = " ".join(para.split())
        if not collapsed:
            continue
        for chunk in _SENTENCE_BOUNDARY_RE.split(collapsed):
            chunk = chunk.strip()
            if chunk:
                sentences.append(chunk)
    return sentences
