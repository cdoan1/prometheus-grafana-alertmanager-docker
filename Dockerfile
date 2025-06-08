FROM golang:1.24-bullseye

WORKDIR /app
 
COPY go.mod ./
COPY go.sum ./
 
RUN go mod download

COPY *.go ./
COPY index.html ./
 
RUN go build -o /server
 
EXPOSE 8090
 
CMD [ "/server" ]