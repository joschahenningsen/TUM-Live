FROM node:16.3.0 as node

WORKDIR /app
COPY web web

## remove generated files in case the developer build with npm before
RUN rm -rf web/assets/ts-dist
RUN rm -rf web/assets/css-dist

WORKDIR /app/web
RUN npm i --no-dev

FROM golang:1.16-alpine as build-env
RUN mkdir /gostuff
WORKDIR /gostuff
COPY go.mod .
COPY go.sum .

# Get dependencies - will also be cached if we won't change mod/sum
RUN go mod download

WORKDIR /go/src/app
COPY . .
COPY --from=node /app/web/assets ./web
COPY --from=node /app/web/node_modules ./web

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /go/bin/server app/server/main.go

FROM alpine
RUN apk add --no-cache tzdata
WORKDIR /app
COPY --from=build-env /go/bin/server .
CMD ["sh", "-c", "sleep 3 && ./server"]
