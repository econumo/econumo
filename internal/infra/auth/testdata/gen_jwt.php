<?php
// gen_jwt.php — produces a REAL RS256 JWT signed with the repo's Lexik private
// key, replicating the exact header/payload shape Lexik + Econumo emit on login.
// Run from repo root via:
//   docker run --rm -v "$PWD":/app -w /app php:8.5-rc-cli \
//     php go/internal/infra/auth/testdata/gen_jwt.php
//
// It also self-verifies the produced token with the public key (openssl_verify)
// to prove the keypair round-trips, then prints a JSON fixture to stdout.

declare(strict_types=1);

$privPath   = 'config/jwt/private.pem';
$pubPath    = 'config/jwt/public.pem';
$passphrase = 'd78eedcb16c13bd949ede5d1b8b910cd';

$priv = openssl_pkey_get_private(file_get_contents($privPath), $passphrase);
if ($priv === false) {
    fwrite(STDERR, "failed to load private key: " . openssl_error_string() . "\n");
    exit(1);
}
$pub = openssl_pkey_get_public(file_get_contents($pubPath));
if ($pub === false) {
    fwrite(STDERR, "failed to load public key: " . openssl_error_string() . "\n");
    exit(1);
}

function b64url(string $data): string
{
    return rtrim(strtr(base64_encode($data), '+/', '-_'), '=');
}

// Fixed iat so the fixture is deterministic. exp = iat + 2592000 (Lexik TTL).
$iat = 1700000000;
$ttl = 2592000;
$exp = $iat + $ttl;

$userId = '01890a5d-ac96-774b-8e3f-9d1b2c3d4e5f';
$email  = 'alice@example.com';

// Header: Lexik/php-jwt default for RS256.
$header = ['typ' => 'JWT', 'alg' => 'RS256'];

// Payload: Lexik base (iat, exp, roles, username) merged with Econumo's
// AuthenticationUpdateTokenPayload additions (id + username=decoded email).
// Lexik's user_identity_field defaults to "username"; Econumo overwrites it
// with the decoded plaintext email, which is what we replicate.
$payload = [
    'iat'      => $iat,
    'exp'      => $exp,
    'roles'    => ['ROLE_USER'],
    'username' => $email,
    'id'       => $userId,
];

$segments = [
    b64url(json_encode($header, JSON_UNESCAPED_SLASHES)),
    b64url(json_encode($payload, JSON_UNESCAPED_SLASHES)),
];
$signingInput = implode('.', $segments);

$signature = '';
if (!openssl_sign($signingInput, $signature, $priv, OPENSSL_ALGO_SHA256)) {
    fwrite(STDERR, "openssl_sign failed: " . openssl_error_string() . "\n");
    exit(1);
}
$segments[] = b64url($signature);
$jwt = implode('.', $segments);

// Self-verify with the public key (this is exactly how Lexik would verify a
// token from the Go Issue() method too — same key, same RS256).
$ok = openssl_verify($signingInput, $signature, $pub, OPENSSL_ALGO_SHA256);
if ($ok !== 1) {
    fwrite(STDERR, "openssl_verify failed (ok=$ok): " . openssl_error_string() . "\n");
    exit(1);
}

echo json_encode([
    'token' => $jwt,
    'expected' => [
        'iat'      => $iat,
        'exp'      => $exp,
        'roles'    => ['ROLE_USER'],
        'username' => $email,
        'id'       => $userId,
    ],
], JSON_PRETTY_PRINT | JSON_UNESCAPED_SLASHES) . "\n";
