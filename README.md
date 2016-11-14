# docker-mesos
A simple mesos framework for running docker containers on mesos.

How to build the binary?
- Just run make and it will build the docker binary for you.

`bin/docker-mesos --help`

NAME:
   docker-mesos - Runs docker containers on mesos.

USAGE:
   docker-mesos [global options] command [command options] [arguments...]

VERSION:
   0.0.1

COMMANDS:
     help, h  Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --copies value, -c value                  Specify the number of copies to run. (default: 1)
   --cpus value                              Maximum cpus required for task. (default: 0.1)
   --memory value                            Maximum memory to allocate to the task. (default: 500)
   --mesos_authentication_secret_file value  Absolute path for mesos authentication file.
   --mesos_authentication_principal value    Use a logrus hook
   --mesos_authentication_provider value     Some usage (default: "SASL")
   --mesos-master value                      Mesos master ip and port (default: "10.85.5.50:5050")
   --help, -h                                show help
   --version, -v                             print the version

Example command:
bin/docker-mesos run ubuntu:16.04 /bin/sleep 60

Notice that above command replaces docker with docker-mesos.
