import json
import subprocess

import click
import requests

COMMAND = ["pass", "show", "hybrid/quizlet_key"]


def _sync_cards(cards, cid):
    token = subprocess.check_output(COMMAND).decode("utf-8").strip()
    headers = {"Authorization": "Bearer {}".format(token)}
    endpoint = "https://api.quizlet.com/2.0/sets/{}/terms".format(cid)
    terms = requests.get(endpoint, headers=headers)

    for oid in [o['id'] for o in terms.json()]:
        print("deleting", oid)
        requests.delete("{}/{}".format(endpoint, oid), headers=headers)

    for card in cards:
        print(card['back'])
        if 'front' in card:
            resp = requests.post(
                endpoint,
                headers=headers,
                data={
                    "term": card['front'],
                    "definition": card['back'],
                })
        if 'img-id' in card:
            desc = "Image of {}".format(card['back']) if \
                   'front' in card else card['back']
            resp = requests.post(
                endpoint,
                headers=headers,
                data={
                    "term": desc,
                    "image": card['img-id'],
                })
        # TODO verify status code (site is giving 500s right now)


@click.argument('path')
@click.argument('cid')
def sync(path, cid):
    """
    PATH is the path to a JSON list file with quizgen cards
    CID is the id for the course in quizlet
    """
    with open(path) as f:
        cards = json.load(f)

    _sync_cards(cards, cid)
