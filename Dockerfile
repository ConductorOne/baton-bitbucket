FROM gcr.io/distroless/static-debian11:nonroot
ENTRYPOINT ["/baton-bitbucket"]
COPY baton-bitbucket /