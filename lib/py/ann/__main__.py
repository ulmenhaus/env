"""
ann: an object annotation tool

ann basically acts as a key-value store on top of your file system where the
key can be a full file path or URL. Thus ann can be a useful building block for
making tooling around annotating documents and websites.

Potential future features:
- listing annotations
- tagging annotations
"""

import base64
import os
import subprocess
import sys

import click


def encode_key(key):
    # tag with version 1
    return b"1" + base64.b64encode(key.encode("utf8")).replace(b"/", b"-")


def decode_key(key):
    tag = key[:1]
    if tag != b'1':
        raise ValueError("Unknown encoding version", tag)
    untagged = key[1:]
    return base64.b64decode(untagged.replace(b"-", b"/")).decode("utf8")


@click.group()
def cli():
    pass


def ls():
    # TODO support tagging (maybe with sfs)
    for folder in os.listdir(os.environ['ANN_DIR']):
        print(decode_key(folder.encode('ascii')))


@click.argument('url')
def ed(url):
    editor = os.environ.get("EDITOR", "emacs")
    wd = os.path.join(os.environ['ANN_DIR'], encode_key(url).decode("ascii"))
    os.mkdir(wd)
    # would be good to do an execv here instead
    subprocess.call([editor, os.path.join(wd, "README.md")])


def main():
    cli.command(name='ed')(ed)
    cli.command(name='ls')(ls)
    cli(obj={})


if __name__ == "__main__":
    main()
