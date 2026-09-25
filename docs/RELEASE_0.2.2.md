# Chameleon Protocol 0.2.2

Версия 0.2.2 посвящена качеству проекта, а не новым transport-возможностям.

Добавлено:

- CodeQL;
- Dependabot;
- CODEOWNERS;
- PR template и bug template;
- fuzz seed для binary frame parser;
- проверяемый release process;
- source-only GitHub Release workflow.

Изменено:

- CI переведён на актуальные official GitHub Actions;
- README синхронизирован с Go 1.25 и текущей версией;
- CONTRIBUTING теперь требует race tests для concurrent/security state;
- SECURITY описывает конкретные классы ошибок, важные для проекта.

Модель безопасности не менялась. Проект остаётся исследовательским прототипом.
