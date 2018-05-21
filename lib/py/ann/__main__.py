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
import re
import shutil
import subprocess
import sys

import click

from git import Repo


def shortname(url):
    return "s{}".format(hashlib.sha256(url.encode("utf-8")).hexdigest()[:6])


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
    sd = os.path.join(os.environ['ANN_DIR'], shortname(url))
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
    sd = os.path.join(os.environ['ANN_DIR'], shortname(url))
    if os.path.exists(sd):
        shutil.rmtree(sd)


@click.argument('url')
def wd(url):
    long_name = encode_key(url).decode("ascii")
    wd = os.path.join(os.environ['ANN_DIR'], long_name)
    if not os.path.exists(wd):
        os.mkdir(wd)
    sd = os.path.join(os.environ['ANN_DIR'], shortname(url))
    if not os.path.exists(sd):
        os.symlink(long_name, sd)
    print(sd)


def this():
    """
    assuming the current directory is for a github repo and it's
    on a branch ISS-<num> where github.com/<namespace>/<repo>/issues/<num>
    is the corresponding issue for the branch, output the issue URL
    """
    wd = os.getcwd()
    loc, shortwd = wd.split("github.com", 1)
    # XXX not for windows
    parts = shortwd.split("/")
    namespace, reponame = parts[1:3]
    rootpath = os.path.join(loc, "github.com", namespace, reponame)
    repo = Repo(rootpath)
    branch = repo.head.ref
    match = re.compile("^ISS-(\d+)$").match(branch.name)
    if not match:
        print("branch does not have a known pattern", file=sys.stderr)
        exit(1)
    number, = match.groups()
    print("https://github.com/{}/{}/issues/{}".format(namespace, reponame,
                                                      number))


def main():
    # TODO urls to local files should be normalized by adding abspath
    # and stripping machine specific prefixes
    cli.command(name='ed')(ed)
    cli.command(name='ls')(ls)
    cli.command(name='rm')(rm)
    cli.command(name='wd')(wd)
    cli.command(name='this')(this)
    cli(obj={})


if __name__ == "__main__":
    main()
