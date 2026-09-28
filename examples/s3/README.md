# S3 example

## Quick start

Set the S3 location first. The example uses the AWS SDK default credential
chain:

```sh
export AWS_REGION=us-east-1
export SUNDIAL_S3_BUCKET=my-config-bucket
export SUNDIAL_S3_CURRENT_REVISION_KEY=production/app/metadata.yaml
export SUNDIAL_S3_REVISION_KEY_PREFIX=production/app/

go run ./examples/s3 -init ./examples/s3/config.yaml
```

`-init` publishes the file before loading it. It is only needed when creating
or resetting the example configuration.

Update the port with a conditional write:

```sh
go run ./examples/s3 -port 9090
```

The update is conditional: it fails with a conflict if another writer publishes
a revision after this process loads the configuration.

The example uses the YAML codec. `metadata.yaml` contains `current_revision_id`;
`<revision-id>.yaml` stores the original business configuration.
Revision IDs use ULID. S3 user metadata uses `parent-id` to link each revision
to its parent.

This example explicitly passes `func(v config) config { return v }` for `clone`
because its configuration contains only value fields. A nil `clone` is rejected.
If you add maps, slices or pointers, update the function to deep-copy their data.
