# S3 example

## Quick start

Create the S3 object first using [config.json](config.json). Sundial treats a
missing object as an error and does not create an empty configuration.

Then run the example from the repository root. The S3 Provider uses the AWS SDK
default credential chain:

```sh
export AWS_REGION=us-east-1
export SUNDIAL_S3_BUCKET=my-config-bucket
export SUNDIAL_S3_PATH_PREFIX=production
export SUNDIAL_S3_KEY=app.json

go run ./examples/s3
```

Update the port with a conditional write:

```sh
go run ./examples/s3 -port 9090
```

The update succeeds only if the object has not changed since it was loaded. A
concurrent update is reported as a conflict and is not retried automatically.
