# bench/baselines/ — resultados publicados

Artefatos de baseline versionados, um diretório por execução publicada. Eles
existem para que `BENCHMARKS.md` tenha números reproduzíveis e não apenas uma
narrativa.

Regras:

- Um diretório é **imutável** depois de publicado. Uma correção gera um novo
  diretório e uma nota em `BENCHMARKS.md`.
- Todo baseline declara commit, hardware, `config_hash`, workload, duração e
  veredito. O `report.json` é o dado bruto; o `report.md` é a leitura humana.
- Pré-baselines de harness (por exemplo, um sink OTLP local) são rotulados como
  tal e **não** são evidência do Gate G1.
- Resultados de cluster T1 só entram aqui depois de executados no cluster
  provisionado e com o commit registrado.
- `bench/results/` e `bench/reports/` continuam fora do git; só o consolidado
  publicado é copiado para cá.
