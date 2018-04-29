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
import hashlib
import os
import shutil
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
        if folder.startswith("s"):
            # ignore short names
            continue
        print(decode_key(folder.encode('ascii')))


@click.argument('url')
def ed(url):
    editor = os.environ.get("EDITOR", "emacs")
    long_name = encode_key(url).decode("ascii")
    wd = os.path.join(os.environ['ANN_DIR'], long_name)
    if not os.path.exists(wd):
        os.mkdir(wd)
    short_name = "s{}".format(
        hashlib.sha256(url.encode("utf-8")).hexdigest()[:6])
    sd = os.path.join(os.environ['ANN_DIR'], short_name)
    if not os.path.exists(sd):
        os.symlink(long_name, sd)
    # would be good to do an execv here instead
    subprocess.call([editor, os.path.join(sd, "README.md")])


@click.argument('url')
def rm(url):
    long_name = encode_key(url).decode("ascii")
    wd = os.path.join(os.environ['ANN_DIR'], long_name)
    if os.path.exists(wd):
        shutil.rmtree(wd)
    short_name = "s{}".format(
        hashlib.sha256(url.encode("utf-8")).hexdigest()[:6])
    sd = os.path.join(os.environ['ANN_DIR'], short_name)
    if os.path.exists(sd):
        shutil.rmtree(sd)


def main():
    cli.command(name='ed')(ed)
    cli.command(name='ls')(ls)
    cli.command(name='rm')(rm)
    cli(obj={})


if __name__ == "__main__":
    main()
