FROM ubuntu:14.04

RUN apt-get update && apt-get install -y python-dev python-m2crypto python-pip

RUN echo "cd /highland" > /bash_init

COPY /requirements.txt /requirements.txt

RUN pip install -r /requirements.txt

RUN pip install --upgrade six

ENTRYPOINT ["bash", "--init-file", "/bash_init"]