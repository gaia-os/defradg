# syntax=docker/dockerfile:1

# An image to run defradb.

# Stage: BUILD
# Several steps are involved to enable caching and because of the behavior of COPY regarding directories.
FROM docker.io/golang:1.19 AS BUILD
WORKDIR /repo/
COPY go.mod go.sum Makefile ./
RUN make deps:modules
COPY . .
RUN make build

RUN echo "export PATH=$PATH:/etc/profile" >> ~/.bashrc

# Stage: RUN
#FROM gcr.io/distroless/base-debian11
FROM ubuntu:latest



COPY --from=BUILD /repo/build/defradb /defradb

COPY --from=BUILD /repo/examples /usr/defra_examples



# Documents which ports are normally used.
# To publish the ports: `docker run -p 9181:9181` ...

EXPOSE 9161
EXPOSE 9171
EXPOSE 9181



COPY run_defradb_create.sh .
RUN chmod +x /run_defradb_create.sh
CMD ["/run_defradb_create.sh"]



# Default command provided for convenience.
# e.g. docker run -p 9181:9181 source/defradb  start --url 0.0.0.0:9181
#ENTRYPOINT [ "/defradb" ]
