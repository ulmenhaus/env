import os
import time


def watch_for_update(filepath, poll_interval=2):
    update_time = None
    while True:
        time.sleep(poll_interval)
        try:
            new_time = os.stat(filepath).st_mtime
        except FileNotFoundError:
            continue
        if new_time != update_time:
            update_time = new_time
            # HACK might still be writing the file so sleep for another 100ms
            time.sleep(.1)
            yield
