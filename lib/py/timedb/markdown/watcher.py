import curses
import subprocess
import sys

from PIL import Image, ImageOps
from selenium import webdriver

from timedb import schema
# TODO we probably don't want to depend on the virtual gateway package here
from timedb.virtual_gateway import common

from jql import jql_pb2

WATCHER_PATH = ".jql.timedb.markdown_watch"


class Watcher(object):

    def __init__(self, client):
        self.client = client

    def _init_selenium_driver(self):
        options = webdriver.ChromeOptions()
        options.add_argument('--headless')  # Run in headless mode if needed
        options.add_argument('--disable-gpu')  # Disable GPU for headless mode
        return webdriver.Chrome(options=options)

    def _clear_terminal(self):
        curses.setupterm()
        sys.stdout.write(curses.tigetstr("clear").decode())
        sys.stdout.flush()

    def _display_image(self, filename):
        subprocess.run(['imgcat', filename])

    # TODO for now I just render notes, but I should be able to render arbitrary attributes
    def _retrieve_notes(self, pk):
        rel_request = jql_pb2.ListRowsRequest(
            table=schema.Tables.Assertions,
            order_by=schema.Fields.Order,
            conditions=[
                jql_pb2.Condition(requires=[
                    jql_pb2.Filter(
                        column=schema.Fields.Arg0,
                        equal_match=jql_pb2.EqualMatch(value=pk),
                    ),
                    jql_pb2.Filter(
                        column=schema.Fields.Relation,
                        equal_match=jql_pb2.EqualMatch(value=".Note"),
                    ),
                ]),
            ],
        )
        assertions = self.client.ListRows(rel_request)
        cmap = {c.name: i for i, c in enumerate(assertions.columns)}
        primary = common.get_primary(assertions)
        notes = []
        for row in assertions.rows:
            note = row.entries[cmap[schema.Fields.Arg1]].formatted
            if note.startswith("#"):
                notes.append(note)
            else:
                notes.append(f"* {note}")
        return notes

    def watch_forever(self):
        driver = self._init_selenium_driver()
        for line in sys.stdin:
            pk = line.strip()
            print("rendering", pk)
            markdown = "## Notes\n" + "\n".join(self._retrieve_notes(pk))
            encoded = markdown.encode("utf-8").hex()
            url = "http://localhost:9070/" + line.strip() + "?raw=" + encoded
            try:
                driver.get(url)
            except Exception as e:
                print("Error rendering", e)
            # TODO should probably use tmpfile for this instead of some fixed path
            path = "/tmp/markdown.png"
            # clear off the table-of-contents and header
            driver.execute_script(
                "elem = document.getElementById('ui-toc-affix'); elem.parentNode.removeChild(elem); window.scrollBy(0, 100);"
            )
            driver.save_screenshot(path)
            image = Image.open(path)
            image = ImageOps.invert(image)
            bbox = image.getbbox()
            if bbox:
                image = image.crop(bbox)
            image.save(path)
            self._clear_terminal()
            self._display_image("/tmp/markdown.png")
