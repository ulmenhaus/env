import collections
import json
import os
import requests

import click
import tabulate

from html.parser import HTMLParser

URL_BASE = "https://en.wikipedia.org/wiki/{}"


# TODO(rabrams) this is specific to certain pages
# would be good to be able to take in a schema for how to translate
# the HTML page to data
class WikiTableParser(HTMLParser):
    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self.in_big = False
        self.in_link = False
        self.last_image = ""
        self.store = []

    def handle_starttag(self, tag, attrs):
        attrs = dict(attrs)
        if tag == "big":
            self.in_big = True
        if tag == "a":
            self.in_link = True
        if tag == "img":
            self.last_image = "http:{}".format(attrs['src'])

    def handle_endtag(self, tag):
        if tag == "big":
            self.in_big = False
        if tag == "a":
            self.in_link = False

    def handle_data(self, data):
        if self.in_big and self.in_link:
            self.store.append((data, self.last_image))


@click.argument('page')
@click.option(
    '--pretty',
    is_flag=True,
    default=False,
    help='If true, print data as a table.')
def scrape(page, pretty):
    """
    PAGE should be the last part of a Wikipedia list page
    """
    resp = requests.get(URL_BASE.format(page))
    body = resp.text
    parser = WikiTableParser()
    parser.feed(body)
    out = []
    for i in range(len(parser.store)):
        ordinal = i + 1
        name, src = parser.store[i]
        out.append({
            "front": str(ordinal),
            "back": name,
            "img-src": src,
        })
    if pretty:
        keys = ["front", "back", "image"]
        tabulated = collections.defaultdict(list)
        for item in out:
            for key in keys:
                tabulated[key].append(item[key])
        print(tabulate.tabulate(tabulated, headers="keys"))
    else:
        print(json.dumps(out, indent=4))
