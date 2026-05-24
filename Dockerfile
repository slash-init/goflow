FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY bin/goflow .

RUN addgroup -S goflow && adduser -S goflow -G goflow
USER goflow

EXPOSE 8080

ENTRYPOINT ["./goflow"]