"""Align a target phoneme sequence against a recognized (actual) one.

Standard edit-distance alignment, the same algorithm used to compute
phone error rate (PER) in speech research -- but kept here with a
backtrace so the practice UI can render each phone as a match,
substitution, insertion (you produced an extra phone), or deletion (you
dropped one), rather than collapsing straight to a single error count.
"""

from dataclasses import dataclass
from enum import Enum
from typing import List, Optional, Sequence


def normalize_phone(phone: str) -> str:
    """Strip stress (ˈˌ) and length (ː) marks so target phonemes (from
    espeak G2P, which marks stress) compare cleanly against recognized
    phonemes (from the CTC model, which doesn't)."""
    return "".join(c for c in phone if c not in "ˈˌː")


class Op(Enum):
    MATCH = "match"
    SUBSTITUTE = "substitute"
    INSERT = "insert"  # present in recognized, absent from target
    DELETE = "delete"  # present in target, absent from recognized


@dataclass
class AlignedPhone:
    op: Op
    target: Optional[str]
    recognized: Optional[str]
    target_index: Optional[int] = None  # index into the original `target` sequence
    recognized_index: Optional[int] = None  # index into `recognized`


def align_phonemes(
    target: Sequence[str], recognized: Sequence[str]
) -> List[AlignedPhone]:
    """Levenshtein-align `target` to `recognized`, substitution cost 1."""
    n, m = len(target), len(recognized)
    # cost[i][j] = edit distance between target[:i] and recognized[:j]
    cost = [[0] * (m + 1) for _ in range(n + 1)]
    for i in range(1, n + 1):
        cost[i][0] = i
    for j in range(1, m + 1):
        cost[0][j] = j
    for i in range(1, n + 1):
        for j in range(1, m + 1):
            sub_cost = 0 if target[i - 1] == recognized[j - 1] else 1
            cost[i][j] = min(
                cost[i - 1][j - 1] + sub_cost,  # match/substitute
                cost[i - 1][j] + 1,  # delete target[i-1]
                cost[i][j - 1] + 1,  # insert recognized[j-1]
            )

    aligned: List[AlignedPhone] = []
    i, j = n, m
    while i > 0 or j > 0:
        if i > 0 and j > 0 and target[i - 1] == recognized[j - 1]:
            aligned.append(AlignedPhone(Op.MATCH, target[i - 1], recognized[j - 1], i - 1, j - 1))
            i, j = i - 1, j - 1
        elif i > 0 and j > 0 and cost[i][j] == cost[i - 1][j - 1] + 1:
            aligned.append(AlignedPhone(Op.SUBSTITUTE, target[i - 1], recognized[j - 1], i - 1, j - 1))
            i, j = i - 1, j - 1
        elif i > 0 and cost[i][j] == cost[i - 1][j] + 1:
            aligned.append(AlignedPhone(Op.DELETE, target[i - 1], None, i - 1, None))
            i -= 1
        else:
            aligned.append(AlignedPhone(Op.INSERT, None, recognized[j - 1], None, j - 1))
            j -= 1
    aligned.reverse()
    return aligned


def phone_error_rate(target: Sequence[str], recognized: Sequence[str]) -> float:
    """Fraction of target phones not matched (substitutions + deletions,
    normalized by target length) -- insertions don't count against the
    target-length denominator, matching the standard PER definition."""
    if not target:
        return 0.0 if not recognized else 1.0
    aligned = align_phonemes(target, recognized)
    errors = sum(1 for a in aligned if a.op in (Op.SUBSTITUTE, Op.DELETE))
    return errors / len(target)
