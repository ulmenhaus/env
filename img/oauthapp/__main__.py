import os

from requests_oauthlib import OAuth2Session

from flask import Flask, request, redirect, session, url_for
from flask.json import jsonify

app = Flask(__name__)

AUTHORIZATION_BASE_URL = os.environ["AUTHORIZATION_BASE_URL"]
CLIENT_ID = os.environ["CLIENT_ID"]
CLIENT_SECRET = os.environ["CLIENT_SECRET"]
TOKEN_URL = os.environ["TOKEN_URL"]


@app.route("/login")
def login():
    osess = OAuth2Session(CLIENT_ID)
    authorization_url, state = osess.authorization_url(AUTHORIZATION_BASE_URL)

    # State is used to prevent CSRF, keep this for later.
    session['oauth_state'] = state
    return redirect(authorization_url)


@app.route("/callback")
def callback():
    osess = OAuth2Session(CLIENT_ID, state=session['oauth_state'])
    token = osess.fetch_token(
        TOKEN_URL,
        client_secret=CLIENT_SECRET,
        authorization_response=request.url.replace("http", "https"))

    return jsonify({
        "token": token,
    })


app.secret_key = 'super secret key'
app.config['SESSION_TYPE'] = 'filesystem'
app.run(host='0.0.0.0')
