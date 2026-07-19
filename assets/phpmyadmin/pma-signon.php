<?php
/**
 * Exchanges a short-lived Servika token for phpMyAdmin signon credentials.
 */
declare(strict_types=1);

session_name('pma_signon');
ini_set('session.cookie_path', '/');
session_start();

$token = isset($_GET['t']) ? (string) $_GET['t'] : '';
if (!preg_match('/^[a-f0-9]{16,128}$/', $token)) {
    http_response_code(400);
    exit('Invalid signon token. Open phpMyAdmin from Servika.');
}

$internalToken = trim((string) @file_get_contents('/etc/servika/pma-internal.token'));
if ($internalToken === '') {
    http_response_code(500);
    exit('phpMyAdmin signon is not configured.');
}

$payload = json_encode(['token' => $token], JSON_THROW_ON_ERROR);
$curl = curl_init('http://127.0.0.1:8080/api/v1/internal/pma-redeem');
if ($curl === false) {
    http_response_code(500);
    exit('phpMyAdmin signon could not be initialized.');
}

curl_setopt_array($curl, [
    CURLOPT_RETURNTRANSFER => true,
    CURLOPT_POST => true,
    CURLOPT_POSTFIELDS => $payload,
    CURLOPT_HTTPHEADER => [
        'Content-Type: application/json',
        'X-Internal-Auth: ' . $internalToken,
    ],
    CURLOPT_CONNECTTIMEOUT => 3,
    CURLOPT_TIMEOUT => 5,
]);
$response = curl_exec($curl);
$status = (int) curl_getinfo($curl, CURLINFO_HTTP_CODE);
curl_close($curl);

if ($status !== 200 || !is_string($response)) {
    http_response_code(401);
    exit('The signon token could not be redeemed. Open phpMyAdmin from Servika again.');
}

$data = json_decode($response, true);
if (!is_array($data)
    || !is_string($data['username'] ?? null)
    || !is_string($data['password'] ?? null)
    || !is_string($data['db'] ?? null)
) {
    http_response_code(500);
    exit('The signon service returned an invalid response.');
}

session_regenerate_id(true);
$_SESSION['PMA_single_signon_user'] = $data['username'];
$_SESSION['PMA_single_signon_password'] = $data['password'];
$_SESSION['PMA_single_signon_host'] = 'localhost';
$_SESSION['PMA_single_signon_only_db'] = [$data['db']];
session_write_close();

header('Location: /pma/', true, 302);
exit;
