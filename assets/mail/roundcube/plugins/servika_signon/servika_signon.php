<?php
/**
 * Signs a mailbox in to Roundcube with a one-time Servika token.
 *
 * The panel posts the token to /webmail/index.php. Nothing is read from the
 * query string, so the token cannot reach browser history or a proxy log.
 */
declare(strict_types=1);

class servika_signon extends rcube_plugin
{
    /** This plugin has no reason to load outside the login task. */
    public $task = 'login';

    public function init()
    {
        $this->add_hook('startup', [$this, 'startup']);
        $this->add_hook('authenticate', [$this, 'authenticate']);
    }

    /**
     * Force the login action when a signon token is present, so the panel does
     * not have to depend on Roundcube's own form fields.
     *
     * @param array $args
     * @return array
     */
    public function startup($args)
    {
        if ($this->token() !== '') {
            $args['action'] = 'login';
        }

        return $args;
    }

    /**
     * Supply the credentials for a token-carrying request.
     *
     * @param array $args
     * @return array
     */
    public function authenticate($args)
    {
        $token = $this->token();
        if ($token === '') {
            return $args;
        }

        $credentials = $this->redeem($token);
        if ($credentials === null) {
            $args['abort'] = true;
            $args['error'] = 'The webmail signon token could not be redeemed. Open webmail from Servika again.';

            return $args;
        }

        $args['user'] = $credentials['username'];
        $args['pass'] = $credentials['password'];
        // The panel opens webmail in a new tab, so there is no earlier Roundcube
        // page to have set a test cookie or issued a form token. The request is
        // instead vouched for by the single-use token redeemed above.
        $args['cookiecheck'] = false;
        $args['valid'] = true;

        return $args;
    }

    /**
     * Read the signon token out of the POST body, or an empty string.
     */
    private function token(): string
    {
        if (($_SERVER['REQUEST_METHOD'] ?? '') !== 'POST') {
            return '';
        }
        $token = isset($_POST['_servika_token']) ? (string) $_POST['_servika_token'] : '';

        return preg_match('/^[a-f0-9]{16,128}$/', $token) === 1 ? $token : '';
    }

    /**
     * Exchange the token for the master credential over the loopback.
     *
     * @return array{username: string, password: string}|null
     */
    private function redeem(string $token): ?array
    {
        $internalToken = trim((string) @file_get_contents('/etc/servika/pma-internal.token'));
        if ($internalToken === '') {
            return null;
        }

        $payload = json_encode(['token' => $token]);
        if (!is_string($payload)) {
            return null;
        }

        $curl = curl_init('http://127.0.0.1:8080/api/v1/internal/webmail-redeem');
        if ($curl === false) {
            return null;
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
            return null;
        }

        $data = json_decode($response, true);
        if (!is_array($data)
            || !is_string($data['username'] ?? null)
            || !is_string($data['password'] ?? null)
        ) {
            return null;
        }

        return ['username' => $data['username'], 'password' => $data['password']];
    }
}
