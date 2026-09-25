# Security Policy

## Supported version

Security fixes делаются для последней версии в `main`.

## Что считать важной security-ошибкой

Например:

- обход authenticated handshake;
- тихая замена pinned identity;
- повторное использование или неправильная derivation ключей;
- panic/crash на недоверенном frame;
- data race в security/session state;
- запись private key с небезопасными правами;
- подмена runtime path через сохранённый JSON.

## Как сообщить

Если в репозитории доступен **Security -> Report a vulnerability**, используйте
его. Не публикуйте рабочие секреты, private keys и подробный exploit в обычном
issue.

Для воспроизведения достаточно:

1. commit SHA;
2. минимального входа;
3. ожидаемого результата;
4. фактического результата;
5. версии Go и ОС.

## Ограничения проекта

Chameleon Protocol — исследовательский прототип. Он не проходил независимый
криптографический аудит и не должен рассматриваться как замена WireGuard, TLS
или QUIC в production.
