# firetap

firetap is an AWS Lambda extension to transport logs to Kinesis Firehose(TODO).

XXX: This is a work in progress.

## Architecture

1. Run `firetap` as an AWS Lambda extension.
   - Runs server that listens to the UNIX domain socket `/tmp/firetap.sock`.
   - Reads logs from the socket and sends them to Kinesis Firehose.
2. Run `firetap` as a bootstrap wrapper command.
   - Invokes `handler` as a subprocess (This is YOUR Lambda function).
   - Reads the standard output of the subprocess and sends it to the extension.

## Usage

### Lambda Extension

`firetap` works as a Lambda extension.

To deploy `firetap` as a Lambda extension, you need to create a Lambda layer that contains `firetap` binary in `extensions/` directory.

```console
$ mkdir extensions
$ cp /path/to/firetap extensions/firetap
$ zip -r layer.zip extensions
$ aws lambda publish-layer-version \
		--layer-name firetap \
		--zip-file fileb://layer.zip \
		--compatible-runtimes provided.al2023 provided.al2
```

### Bootstrap Wrapper

You can use `firetap` as a bootstrap wrapper command.

When the `firetrap` binary runs as a Lambda runtime, it runs your Lambda function as a subprocess and sends the standard output of the subprocess to the extension via the UNIX socket.

You simply need to place the `firetrap` binary as `bootstrap` in the Lambda package. Your Lambda function should be placed as a `handler` (the lambda function configuration `.Handler` element) in the same directory.

```
.
├── bootstrap
└── handler
```

## LICENSE

MIT

## Author

Fujiwara Shunichiro
