FROM golang:1.24-bullseye

WORKDIR /app
 
ARG GIT_VERSION
ARG GIT_COMMIT

COPY go.mod ./
COPY go.sum ./
 
RUN go mod download

COPY *.go ./
COPY index.html ./
 
# RUN go build -o /server
RUN go build -ldflags="-X 'main.buildVersion=${GIT_VERSION}' -X 'main.buildCommit=${GIT_COMMIT}'" -o /server ./server.go

EXPOSE 8090
 
CMD [ "/server" ]