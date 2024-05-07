# firetap

firetap is an AWS Lambda extension to transport logs to Kinesis Data Firehose or Kinesis Data Streams.

This is an alpha version and not recommended for production use.

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

#### Configurations

You can configure `firetap` by setting environment variables.

- `FIRETAP_STREAM_NAME`: The name of the Kinesis Data Firehose or Kinesis Data Streams stream.
- `FIRETAP_USE_DATA_STREAM`: Set `true` if you want to use Kinesis Data Streams. Default is `false` (use Firehose).

### Bootstrap Wrapper

`firetap` is also designed to work as a bootstrap wrapper command.

When the `firetrap` binary runs as a Lambda runtime, it runs your Lambda function as a subprocess and sends the standard output of the subprocess to the extension via the UNIX socket. The standard error of the subprocess is not sent to the extension.

You can prepare `bootstrap` shell script as below. Your Lambda function should be placed as a `handler` (the lambda function configuration `.Handler` element) in the same directory.

```sh
#!/bin/sh
exec /opt/extensions/firetap
```

```
.
├── bootstrap
└── handler
```

## LICENSE

MIT

## Author

Fujiwara Shunichiro
