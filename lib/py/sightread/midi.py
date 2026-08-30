"""A minimal Standard MIDI File (format 0) writer -- just enough to turn a
generated exercise into a .mid a real synth can play. No third-party MIDI
library is used; this hand-rolls the handful of chunks/events we need.
"""

import struct

from sightread import theory

GUITAR_NYLON_PROGRAM = 24
TICKS_PER_BEAT = 480


def _vlq(value):
    """MIDI variable-length quantity encoding."""
    bytes_out = [value & 0x7F]
    value >>= 7
    while value:
        bytes_out.append((value & 0x7F) | 0x80)
        value >>= 7
    return bytes(reversed(bytes_out))


def timed_events(systems):
    """Yield (start_beat, beats, notes) for every event, in playback order."""
    t = 0.0
    for measures in systems:
        for events in measures:
            for event in events:
                yield t, event['beats'], event['notes']
                t += event['beats']


def write_midi(systems, out_path, bpm=76, program=GUITAR_NYLON_PROGRAM):
    events = []  # (abs_tick, priority, bytes) -- priority sorts note-off before note-on
    for start_beat, beats, notes in timed_events(systems):
        tick = round(start_beat * TICKS_PER_BEAT)
        dur_ticks = round(beats * TICKS_PER_BEAT)
        gap = max(1, int(dur_ticks * 0.08))
        sound_ticks = max(1, dur_ticks - gap)
        if notes:
            velocity = 78 if len(notes) == 1 else 68
            for letter, acc, octave in notes:
                pitch = theory.midi_number(letter, acc, octave)
                events.append((tick, 1, bytes([0x90, pitch, velocity])))
                events.append((tick + sound_ticks, 0, bytes([0x80, pitch, 0])))

    events.sort(key=lambda e: (e[0], e[1]))

    track = bytearray()
    tempo_us = int(60_000_000 / bpm)
    track += _vlq(0) + bytes([0xFF, 0x51, 0x03]) + struct.pack(">I", tempo_us)[1:]
    track += _vlq(0) + bytes([0xC0, program])

    last_tick = 0
    for abs_tick, _, data in events:
        track += _vlq(abs_tick - last_tick) + data
        last_tick = abs_tick
    track += _vlq(0) + bytes([0xFF, 0x2F, 0x00])

    header = b"MThd" + struct.pack(">IHHH", 6, 0, 1, TICKS_PER_BEAT)
    track_chunk = b"MTrk" + struct.pack(">I", len(track)) + bytes(track)

    with open(out_path, "wb") as f:
        f.write(header + track_chunk)

    return out_path
