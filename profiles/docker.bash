function start-workday {
    add-keypairs docker-ssh
    open $PRIVATE_DIR/kdbs/master.kdbx
    for app in Google\ Chrome Mail Slack Spotify
    do
	open "/Applications/$app.app";
    done
}

function work-context {
    # only highland is supported for now
    cd $ORG_DIR/docker/highland
    source venv/bin/activate
}

function docker-enter {
    docker exec -it $1 bash
}

function highland-build-and-tag {
    docker build -t docker/highland:staging .
    docker tag -f docker/highland:staging highland:latest
}

function highland-build-tag-and-run {
    highland-build-and-tag
    docker-compose up
}

function test-context {
    export PYTHONPATH="/Users/caervs/source/org/docker/highland/image/:/Users/caervs/source/org/docker/highland/bootstrap/:$PYTHONPATH"
    alias fablib-cli='python -m fablib.cli --highland_password fPMDBvHffPMDBvHf'
}

function build-yoyobrawlah {
    fablib-cli build --source_url https://github.com/caervs/yoyobrawlah --docker_repo caervs --docker_tag caervs/yoyobrawlah
}

# expose port 80 on docker VM with
# boot2docker ssh -L 8000:localhost:80

# to clear sql database
# docker rm db
# may have to run docker-compose stop first
# and perhaps docker rm highland_redis_1

# get redis IP
# API=$(docker ps | grep api | cut -f 1 -d ' '); docker exec $API cat /etc/hosts | grep redis

# queue new job with
# fab cli_build:https://github.com/caervs/yoyobrawlah,caervs,caervs/yoyobrawlah

# run drain worker locally with
# SETTINGS_FLAVOR=development REDIS_1_PORT_6379_TCP_ADDR=localhost REDIS_1_PORT_6379_TCP_PORT=6379 fab drain_worker


# run highland tests in a container
# docker run -it -v `pwd`/image:/image docker/highland:staging  bash -c "cd /image; trial highland"

# add ssh keys to a linode instance
# fab h:nj-a saltcall:ssh.init.add_company_ssh_keys

# push local code to staging node, build docker image and add to registry
# fab h:kernel-builder project build push:staging
# pull new highland from registry
# fab h:test-1 pull:staging worker api manager

