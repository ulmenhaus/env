import subprocess

from oauth2client import client


def get_oauth_creds():
    """
    Get credentials for an oauth google client
    """
    # TODO(rabrams) too opinionated about pass hierarchy

    return client.Credentials.new_from_json(
        subprocess.check_output(
            ["pass", "show", "hybrid/tokens/gmail.personal.token"]))
