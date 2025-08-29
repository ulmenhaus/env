import subprocess
import urllib.parse


def open_session(prompt, service, model):
    if service == "chatgpt" and model == "4o":
        prompt_url = "https://chatgpt.com/?model=gpt-4o&q=" + urllib.parse.quote(
            prompt)
        subprocess.check_call(["txtopen", prompt_url])
    else:
        raise ValueError(service, model)
