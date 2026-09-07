# S3 example

## Quick start

Set the S3 location first. The example uses the AWS SDK default credential
chain:

```sh
export AWS_REGION=us-east-1
export SUNDIAL_S3_BUCKET=my-config-bucket
export SUNDIAL_S3_CURRENT_REVISION_KEY=production/app/current
export SUNDIAL_S3_REVISION_KEY_PREFIX=production/app/history/

go run ./examples/s3 -init ./examples/s3/config.json
```

`-init` publishes the file before loading it. It is only needed when creating
or resetting the example configuration.

Update the port with a conditional write:

```sh
go run ./examples/s3 -port 9090
```

The update is conditional: it fails with a conflict if another writer publishes
a revision after this process loads the configuration.
