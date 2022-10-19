# Convert a public Ed25519 key (in PEM format) for use in ClusterCockpit

Imagine you have externally generated JSON Web Tokens (JWT) that should be accepted by CC backend. This external provider shares its public key (used for JWT signing) in PEM format:

```
-----BEGIN PUBLIC KEY-----
MCowBQYDK2VwAyEA+51iXX8BdLFocrppRxIw52xCOf8xFSH/eNilN5IHVGc=
-----END PUBLIC KEY-----
```

Unfortunately, ClusterCockpit does not handle this format (yet). You can use this tool to convert the public PEM key into a representation for CC:

```
CROSS_LOGIN_JWT_PUBLIC_KEY="+51iXX8BdLFocrppRxIw52xCOf8xFSH/eNilN5IHVGc="
```

Instructions

- `cd tools/convert-pem-pubkey-for-cc/`
- Insert your public ed25519 PEM key into `dummy.pub`
- `go run . dummy.pub`
- Copy the result into ClusterCockpit's `.env`
- (Re)start ClusterCockpit backend

Now CC can validate generated JWTs from the external provider.
