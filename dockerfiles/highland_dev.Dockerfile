FROM ubuntu:14.04

RUN apt-get update && apt-get install -y python-dev python-m2crypto python-pip iptables pass python-psycopg2

COPY /requirements.txt /requirements.txt

RUN pip install -r /requirements.txt

RUN pip install --upgrade six

RUN echo "eval \$(gpg-agent --daemon --pinentry-program /usr/bin/pinentry); cd /highland; export all=api,agent,newrelic,nginx,builder,heka,hekasink,logbahn" > /bash_init

COPY highland-client /highland-client

COPY highland /highland-static

RUN pip install -e /highland-client

RUN pip install requests==2.9.1

RUN easy_install python-dateutil

RUN echo 1345
RUN cd / && pip install -e git://github.com/caervs/boto.git@subnet_attribute#egg=boto && pip install -e git://github.com/caervs/sshuttle.git@master#egg=sshuttle

RUN cd /src/boto && git checkout -b subnet_attribute origin/subnet_attribute

RUN echo "ln -s /rabrams/.dockercfg /root/.dockercfg" >> /bash_init

ENTRYPOINT ["bash", "--init-file", "/bash_init"]