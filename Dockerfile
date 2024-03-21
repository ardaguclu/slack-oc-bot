FROM registry.ci.openshift.org/ocp/builder:rhel-9-golang-1.21-openshift-4.16 AS builder
WORKDIR /go/src/github.com/ardaguclu/slack-oc-bot
COPY . .
RUN go build .

FROM registry.ci.openshift.org/ocp/4.16:cli AS cli

FROM registry.ci.openshift.org/ocp/4.16:base
COPY --from=cli /usr/bin/oc /usr/bin/
RUN ln -s /usr/bin/oc /usr/bin/kubectl

COPY --from=builder /go/src/github.com/ardaguclu/slack-oc-bot /usr/bin/slack-oc-bot
RUN chmod +x /usr/bin/slack-oc-bot
ENTRYPOINT ["/usr/bin/slack-oc-bot"]