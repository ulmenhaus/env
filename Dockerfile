FROM ubuntu

WORKDIR /ulmenhaus/env

RUN apt-get update

RUN echo 2 | apt-get install -y tzdata

RUN apt-get update && apt-get install -y  \
	build-essential \
	dpic \
	gmt-common \
	m4 \
	pdf2svg \
	python3-pip \
	sudo \
	texlive-binaries \
	texlive-extra-utils \
	texlive-pstricks \
	xzdec

RUN pip3 install markdown

COPY lib /ulmenhaus/env/lib
COPY bin /ulmenhaus/env/bin

ENV PYTHONPATH=/ulmenhaus/env/lib/py
ENV PATH=/ulmenhaus/env/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
