# firetap

firetap is an AWS Lambda extension to transport logs to Kinesis Data Firehose or Kinesis Data Streams.

This is an alpha version and not recommended for production use.

## Architecture

Run `firetap` as an AWS Lambda extension.

- Runs HTTP server that receive telemetries using [Lambda Telemetry API](https://docs.aws.amazon.com/en/lambda/latest/dg/telemetry-api.html).
- Sends function logs to Kinesis Data Firehose / Streams.

### Limitations

The execution of the Lambda function is stopped after it returns the response to the invoker. The telemetry API is called asynchronously, so all logs may not be sent to the Kinesis Data Firehose / Streams if the function is stopped before they are sent.

firetap waits for the SHUTDOWN event to be called before terminating the Lambda function. When the SHUTDOWN event is received, if the remaining logs exist, they are sent to the Kinesis Data Firehose / Streams.

Normally, the SHUTDOWN event is sent after about 5 minutes of the function being idle. So, the logs delayed by up to 5 minutes may be sent. (However, the AWS Lambda specification does not guarantee the delay time.)

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


## LICENSE

MIT

## Author

Fujiwara Shunichiro
