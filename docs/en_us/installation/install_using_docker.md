# Install using docker

## Install && Run

- Run BFE with example configuration files:

```bash
docker run -p 8080:8080 -p 8443:8443 -p 8421:8421 bfenetworks/bfe
```

you can access http://127.0.0.1:8080/ and got status code 500 because of there is rule be matched.
you can access http://127.0.0.1:8421/ got monitor information.

- Run BFE with your configuration files:

```bash
// prepare your configuration (see section Configuration if you need) to dir /Users/BFE/conf

docker run -p 8080:8080 -p 8443:8443 -p 8421:8421 -v /Users/BFE/Desktop/log:/bfe/log -v /Users/BFE/Desktop/conf:/bfe/conf bfenetworks/bfe
```

## Further reading

- Get familiar with [Command options](../operation/command.md)
- Get started with [Beginner's Guide](../example/guide.md)
