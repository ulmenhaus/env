import json
import os
import subprocess

import click
import requests

BATCH_SIZE = 10
COMMAND = ["pass", "show", "hybrid/quizlet_key"]


def _retrieve_image(card):
    back = card["back"]
    img_name = card["front"] if "front" in card else back
    # HACK
    img_name = img_name.replace("/", "_").replace(" ", "_")
    print("Retrieving:", back)
    img_src = card["img-src"]
    img_format = img_src.split(".")[-1]
    img_body = requests.get(img_src).content
    target = os.path.join("images", "{}.{}".format(img_name, img_format))
    with open(target, 'wb') as f:
        f.write(img_body)
    converted = os.path.join("images", "{}.png".format(img_name))
    if img_format != 'png':
        subprocess.check_call(["convert", target, "{}".format(converted)])
    return converted


def _copy_images(cards):
    token = subprocess.check_output(COMMAND).decode("utf-8").strip()
    if not os.path.exists("images"):
        os.mkdir("images")
    paths = []
    for card in cards:
        paths.append(_retrieve_image(card))

    for i in range(0, len(paths), BATCH_SIZE):
        batch_paths = paths[i:i+BATCH_SIZE]
        batch_cards = cards[i:i+BATCH_SIZE]
        # TODO(rabrams) figure out how to do with requests
        args = [
            "curl", "-H", "Authorization: Bearer {}".format(token), "-X", "POST",
            "-F", "whitespace=1"
        ]
        for path in batch_paths:
            args += ["-F", "imageData[]=@{}".format(path)]
        args.append("https://api.quizlet.com/2.0/images")
        out = subprocess.check_output(args)
        for card, dest in zip(batch_cards, json.loads(out)):
            card['img-dest'] = dest['url']
            card['img-id'] = dest['id']


@click.argument('path')
def stage(path):
    """
    PATH is the path to a JSON list file with quizgen cards
    """
    with open(path) as f:
        cards = json.load(f)

    _copy_images(cards)
    with open(path, 'w') as f:
        json.dump(cards, f, indent=4)
