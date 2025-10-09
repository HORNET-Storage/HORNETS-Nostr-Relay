#!/bin/bash

# Change directory
cd services/server/demo

# Build the Go program
go build -o ../../../hornet-storage-demo

# Pause equivalent (wait for user input)
read -p "Press enter to continue"
