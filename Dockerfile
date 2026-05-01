FROM ubuntu

RUN apt-get update

RUN echo 2 | apt-get install -y tzdata

RUN apt-get update && apt-get install -y  \
	build-essential \
  curl \
	docker.io \
	dpic \
	gmt-common \
	golang \
	m4 \
	pdf2svg \
	python3-pip \
	sudo \
	texlive-binaries \
	texlive-extra-utils \
	texlive-pstricks \
	xonsh \
	xzdec

RUN pip3 install --break-system-packages \
  GitPython \
  click \
  gitpython \
  grpcio \
  markdown \
  matplotlib \
  numpy \
  oauth2client \
  pandas \
  prompt-toolkit \
  protobuf \
  python-chess \
  requests \
  scipy \
  tabulate \
  termcolor \
  urwid \
  xontrib-prompt-vi-mode \
  yapf

# Install claude into an isolated home so root's home isn't exposed to
# other users. The install script stores setup files under $HOME/.claude/
# and places the launcher at $HOME/.local/bin/claude.
RUN curl -fsSL https://claude.ai/install.sh | HOME=/opt/claude-home bash && \
    mv /opt/claude-home/.local/bin/claude /usr/local/bin/ && \
    chmod -R a+rX /opt/claude-home

RUN ln -s /usr/bin/python3 /usr/local/bin/python3

WORKDIR /src/github.com/ulmenhaus/env

COPY . /src/github.com/ulmenhaus/env/

ENV PYTHONPATH=/src/github.com/ulmenhaus/env/lib/py:./lib/py
ENV PATH=/src/github.com/ulmenhaus/env/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
ENV GOPATH=/src
