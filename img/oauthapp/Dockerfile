FROM python

RUN pip3 install flask requests_oauthlib
RUN mkdir -p /oauthapp/api
COPY *py /oauthapp/api/

EXPOSE 5000
ENV PYTHONPATH /oauthapp
ENV FLASK_APP /oauthapp/api/__main__.py
ENTRYPOINT ["flask", "run", "--host=0.0.0.0"]
