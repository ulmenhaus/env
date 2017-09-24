import os

import click
import yaml

from mutagen.easyid3 import EasyID3
from pydub import AudioSegment


def _path_for_blob(blob_dir, source):
    if 'path' in source:
        return source['path']
    elif 'hash' in source:
        assert 'extension' in source, "Missing extension"
        return os.path.join(blob_dir, "{}.{}".format(source['hash'],
                                                     source['extension']))
    else:
        raise ValueError("No path for source", source)


@click.option(
    '--every',
    is_flag=True,
    default=False,
    help='If true, builds all targets.')
@click.argument('source')
@click.argument('target', nargs=-1)
def build(every, source, target):
    """
    build a music file with parameters specifed in a yaml

    SOURCE is the source yaml
    TARGET is a target music file
    """
    with open(source) as f:
        songs = yaml.load(f)

    blob_dir = os.environ.get("BLOB_DIR", ".")
    for tgt, song in songs.items():
        if not every and tgt not in target:
            continue
        metadata = song['metadata']
        source = song['source']
        processing = song.get('processing', [])
        print(metadata['title'])
        if source['type'] != 'blob':
            raise ValueError("Unknown source type", source['type'])
        seg = AudioSegment.from_file(_path_for_blob(blob_dir, source))
        for process in processing:
            ptype = process['type']
            if ptype != 'trim':
                raise ValueError("Unknown process type", ptype)
            keep = process['keep']
            if len(keep) == 1:
                seg = seg[keep[0]:]
            elif (len(keep) % 2) == 1:
                raise ValueError("Odd number of trims")
            else:
                original = seg
                seg = seg[keep[0]:keep[1]]
                for i in range(2, len(keep), 2):
                    seg += original[keep[i]:keep[i + 1]]

        path = "{}.mp3".format(tgt)
        seg.export(path, format='mp3')
        song = EasyID3(path)
        for key, value in metadata.items():
            song[key] = value
        song.save()
