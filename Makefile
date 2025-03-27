COMMIT_ID?=$(shell git rev-parse --short HEAD)
VERSION?=v0.0.1-${COMMIT_ID}

local:
	docker build -t langgenius/dify-plugin-daemon:local -f ./docker/local.dockerfile .
