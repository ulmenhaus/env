FROM ubuntu:14.04

RUN apt-get update && apt-get install -y python-dev python-m2crypto python-pip iptables pass

RUN cd / && git clone https://github.com/apenwarr/sshuttle

COPY /requirements.txt /requirements.txt

RUN pip install -r /requirements.txt

RUN pip install --upgrade six

RUN echo "eval \$(gpg-agent --daemon --pinentry-program /usr/bin/pinentry); cd /highland" > /bash_init

ENTRYPOINT ["bash", "--init-file", "/bash_init"]