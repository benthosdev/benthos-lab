#!/bin/sh
# This is a very lame script that will be used until Benthos stops being naughty
# and fully opts into Go modules.
go get github.com/Jeffail/benthos@$1 && echo $1 > benthos_version