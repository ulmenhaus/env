import curses
import subprocess
import sys

from PIL import Image, ImageOps
from selenium import webdriver

from timedb import schema
from timedb.virtual_gateway import common

from jql import jql_pb2

WATCHER_PATH = ".jql.timedb.markdown_watch"


class Watcher(object):

    def __init__(self, client):
        self.client = client
        self.field = None
        self.pk = None
        self.scroll_amt = 0
        self.page_height = 0

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
    def _retrieve_attributes(self, pk, relation):
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
                        equal_match=jql_pb2.EqualMatch(value=relation),
                    ),
                ]),
            ],
        )
        assertions = self.client.ListRows(rel_request)
        cmap = {c.name: i for i, c in enumerate(assertions.columns)}
        primary = common.get_primary(assertions)
        attributes = []
        for row in assertions.rows:
            attribute = row.entries[cmap[schema.Fields.Arg1]].formatted
            if attribute.startswith("#") or len(assertions.rows) == 1:
                attributes.append(attribute)
            else:
                attributes.append(f"* {attribute}")
        return attributes

    def _markdown_to_image(self, markdown, driver, path):
        encoded = markdown.encode("utf-8").hex()
        url = f"http://localhost:9070/?raw={encoded}"
        try:
            driver.get(url)
        except Exception as e:
            print("Error rendering", e)
        # clear off the table-of-contents and header and hide the scrollbar
        offset_px = -(45 + self.scroll_amt)
        print(offset_px)
        js = ';'.join([
            "elem = document.getElementById('ui-toc-affix')",
            "elem.parentNode.removeChild(elem)",
            f"document.documentElement.style.transform = 'translateY({offset_px}px)'",
            "document.body.style.overflow = 'hidden';",
        ])
        try:
            driver.execute_script(js)
        except Exception as e:
            print("Got an excaption", e, file=sys.stderr)
        driver.save_screenshot(path)
        image = Image.open(path)
        image = ImageOps.invert(image)
        _, self.page_height = image.size
        self.page_height -= 300
        bbox = image.getbbox()
        if bbox:
            image = image.crop(bbox)
        image.save(path)

    def _render(self, driver):
        print("rendering", repr(self.pk), repr(self.field))
        markdown = f"## {self.field[1:]}\n" + "\n".join(
            self._retrieve_attributes(self.pk, self.field))
        # TODO should probably use tmpfile for this instead of some fixed path
        path = "/tmp/markdown.png"
        self._markdown_to_image(markdown, driver, path)
        self._clear_terminal()
        self._display_image(path)

    def watch_forever(self):
        driver = self._init_selenium_driver()
        for line in sys.stdin:
            encoded = line.strip()
            send_left = False
            if encoded:
                command, self.pk = encoded.split("\t", 1)
                if command.startswith("send-left "):
                    send_left = True
                    self.field = command[len("send-left "):]
                else:
                    self.field = command
                self.scroll_amt = 0
            else:
                self.scroll_amt += self.page_height
            self._render(driver)
            if send_left:
                subprocess.Popen(["tmux", "select-pane", "-L"]).wait()
