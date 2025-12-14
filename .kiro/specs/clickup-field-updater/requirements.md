# Requirements Document

## Introduction

O sistema ClickUp Field Updater é uma extensão da API ClickUp Excel que adiciona uma interface web para atualização em massa de campos personalizados do ClickUp. O sistema permite que usuários façam upload de arquivos CSV/XLSX, mapeiem colunas para campos personalizados, e acompanhem o progresso da atualização em tempo real através de WebSockets. Inclui autenticação básica, sistema de filas com PostgreSQL, e histórico de operações.

## Glossary

- **Sistema**: ClickUp Field Updater web application
- **Usuario**: Pessoa autenticada que utiliza a interface web
- **Campo_Personalizado**: Custom field do ClickUp identificado por UUID
- **Arquivo_Upload**: Arquivo CSV ou XLSX enviado pelo usuário
- **Mapeamento**: Associação entre coluna do arquivo e campo personalizado do ClickUp
- **Fila_Processamento**: Sistema de queue temporário usando PostgreSQL
- **WebSocket_Conexao**: Conexão em tempo real para atualizações de progresso
- **Historico_Operacao**: Registro das operações de atualização realizadas
- **Token_ClickUp**: Token de autenticação pessoal do ClickUp (formato pk_...)
- **Sessao_Usuario**: Sessão autenticada com credenciais básicas
- **Lista_ClickUp**: Lista de tarefas do ClickUp identificada por ID
- **Relatorio_Excel**: Arquivo Excel gerado com dados das tasks do ClickUp
- **Task_ClickUp**: Tarefa individual do ClickUp com campos nativos e personalizados
- **Workspace_ClickUp**: Workspace do ClickUp contendo spaces, folders e listas
- **Space_ClickUp**: Space do ClickUp contendo folders e listas
- **Folder_ClickUp**: Folder do ClickUp contendo listas
- **Metadados_ClickUp**: Dados hierárquicos cached do ClickUp (workspaces, spaces, folders, listas, campos)
- **Progresso_Rosca**: Indicador visual circular de progresso atualizado via WebSocket

## Requirements

### Requirement 1

**User Story:** Como um usuário, eu quero acessar uma interface web protegida por autenticação básica, para que apenas pessoas autorizadas possam atualizar campos do ClickUp.

#### Acceptance Criteria

1. WHEN um usuário acessa a URL da aplicação THEN o Sistema SHALL exibir uma tela de login com campos de usuário e senha
2. WHEN um usuário fornece credenciais válidas THEN o Sistema SHALL criar uma Sessao_Usuario e redirecionar para o dashboard principal
3. WHEN um usuário fornece credenciais inválidas THEN o Sistema SHALL exibir mensagem de erro e manter na tela de login
4. WHEN uma Sessao_Usuario expira ou é inválida THEN o Sistema SHALL redirecionar automaticamente para a tela de login
5. WHEN um usuário está autenticado THEN o Sistema SHALL exibir um botão de logout visível em todas as páginas

### Requirement 2

**User Story:** Como um usuário autenticado, eu quero gerenciar campos personalizados do ClickUp, para que eu possa visualizar e atualizar a lista de campos disponíveis.

#### Acceptance Criteria

1. WHEN um usuário clica no botão "Atualizar Campos Personalizados" THEN o Sistema SHALL fazer requisição à API do ClickUp e buscar todos os campos personalizados
2. WHEN a busca de campos é bem-sucedida THEN o Sistema SHALL salvar os Campo_Personalizado na tabela do banco de dados
3. WHEN a busca de campos falha THEN o Sistema SHALL exibir mensagem de erro específica ao usuário
4. WHEN campos personalizados são atualizados THEN o Sistema SHALL exibir lista atualizada com ID, nome e tipo de cada campo
5. WHEN não existem campos personalizados THEN o Sistema SHALL exibir mensagem informativa apropriada

### Requirement 3

**User Story:** Como um usuário, eu quero fazer upload de arquivos CSV ou XLSX, para que eu possa importar dados para atualização dos campos personalizados.

#### Acceptance Criteria

1. WHEN um usuário seleciona um arquivo THEN o Sistema SHALL validar se o formato é CSV ou XLSX
2. WHEN o arquivo é válido THEN o Sistema SHALL fazer upload e exibir preview das primeiras 5 linhas
3. WHEN o arquivo é inválido THEN o Sistema SHALL exibir mensagem de erro e rejeitar o upload
4. WHEN o upload é concluído THEN o Sistema SHALL extrair e exibir lista de todas as colunas disponíveis
5. WHEN o arquivo excede 10MB THEN o Sistema SHALL rejeitar o upload com mensagem de limite de tamanho

### Requirement 4

**User Story:** Como um usuário, eu quero mapear colunas do arquivo para campos personalizados, para que o sistema saiba qual coluna corresponde a qual campo do ClickUp.

#### Acceptance Criteria

1. WHEN um arquivo é carregado THEN o Sistema SHALL exibir interface de mapeamento com dropdowns para cada coluna
2. WHEN um usuário seleciona um Campo_Personalizado para uma coluna THEN o Sistema SHALL salvar o Mapeamento temporariamente
3. WHEN existem mapeamentos duplicados THEN o Sistema SHALL exibir aviso e impedir o prosseguimento
4. WHEN todos os mapeamentos obrigatórios são definidos THEN o Sistema SHALL habilitar botão "Iniciar Atualização"
5. WHEN o usuário confirma os mapeamentos THEN o Sistema SHALL validar compatibilidade de tipos de dados

### Requirement 5

**User Story:** Como um usuário, eu quero acompanhar o progresso da atualização em tempo real, para que eu saiba o status atual do processamento.

#### Acceptance Criteria

1. WHEN uma atualização é iniciada THEN o Sistema SHALL estabelecer WebSocket_Conexao com o cliente
2. WHEN o processamento avança THEN o Sistema SHALL enviar atualizações de progresso via WebSocket_Conexao
3. WHEN uma linha é processada com sucesso THEN o Sistema SHALL incrementar contador de sucessos
4. WHEN uma linha falha no processamento THEN o Sistema SHALL incrementar contador de erros e registrar detalhes
5. WHEN o processamento é concluído THEN o Sistema SHALL enviar resumo final com totais de sucesso e erro

### Requirement 6

**User Story:** Como um administrador do sistema, eu quero que as atualizações sejam processadas através de um sistema de filas, para que múltiplas operações possam ser gerenciadas de forma organizada.

#### Acceptance Criteria

1. WHEN uma atualização é solicitada THEN o Sistema SHALL criar entrada na Fila_Processamento no PostgreSQL
2. WHEN um job é adicionado à fila THEN o Sistema SHALL processar jobs na ordem FIFO (First In, First Out)
3. WHEN um job é concluído THEN o Sistema SHALL remover a entrada da Fila_Processamento automaticamente
4. WHEN um job falha THEN o Sistema SHALL marcar como erro e manter na fila por 24 horas antes da limpeza
5. WHEN o sistema é reiniciado THEN o Sistema SHALL retomar processamento de jobs pendentes na fila

### Requirement 7

**User Story:** Como um usuário, eu quero visualizar o histórico de operações realizadas, para que eu possa acompanhar quais atualizações foram feitas anteriormente.

#### Acceptance Criteria

1. WHEN uma operação de atualização é iniciada THEN o Sistema SHALL salvar Historico_Operacao com título e timestamp
2. WHEN o usuário acessa a aba de histórico THEN o Sistema SHALL exibir lista das últimas 50 operações
3. WHEN uma operação é concluída THEN o Sistema SHALL atualizar o status no Historico_Operacao
4. WHEN o histórico excede 1000 registros THEN o Sistema SHALL remover automaticamente registros mais antigos
5. WHEN o usuário clica em uma entrada do histórico THEN o Sistema SHALL exibir detalhes da operação

### Requirement 8

**User Story:** Como um usuário, eu quero configurar meu token pessoal do ClickUp e sincronizar metadados, para que o sistema possa acessar minha conta e carregar a estrutura organizacional.

#### Acceptance Criteria

1. WHEN um usuário insere ou altera Token_ClickUp THEN o Sistema SHALL automaticamente buscar todos os Workspace_ClickUp, Space_ClickUp, Folder_ClickUp, Lista_ClickUp e Campo_Personalizado
2. WHEN a sincronização é iniciada THEN o Sistema SHALL exibir Progresso_Rosca ao lado do botão via WebSocket_Conexao
3. WHEN a sincronização é bem-sucedida THEN o Sistema SHALL armazenar Metadados_ClickUp com ID e nome de cada elemento
4. WHEN a sincronização falha THEN o Sistema SHALL exibir mensagem de erro específica e manter token anterior se existir
5. WHEN os metadados são atualizados THEN o Sistema SHALL atualizar automaticamente os menus suspensos na aba "Buscar Tasks"

### Requirement 9

**User Story:** Como um usuário, eu quero buscar tasks do ClickUp usando seleção hierárquica através da aba "Buscar Tasks", para que eu possa navegar facilmente pela estrutura organizacional.

#### Acceptance Criteria

1. WHEN um usuário acessa a aba "Buscar Tasks" THEN o Sistema SHALL exibir menus suspensos hierárquicos: Workspace, Space, Folder, Lista
2. WHEN um Workspace_ClickUp é selecionado THEN o Sistema SHALL filtrar e exibir apenas Space_ClickUp vinculados ao workspace selecionado
3. WHEN um Space_ClickUp é selecionado THEN o Sistema SHALL filtrar e exibir apenas Folder_ClickUp vinculados ao space selecionado
4. WHEN um Folder_ClickUp é selecionado THEN o Sistema SHALL filtrar e exibir apenas Lista_ClickUp vinculadas ao folder selecionado
5. WHEN listas são selecionadas THEN o Sistema SHALL permitir seleção múltipla de folders e listas e exibir Campo_Personalizado disponíveis para seleção

### Requirement 10

**User Story:** Como um usuário, eu quero fazer upload de arquivos para atualizar campos personalizados através da aba "Uploads", para que eu possa atualizar múltiplas tasks de uma vez.

#### Acceptance Criteria

1. WHEN um usuário acessa a aba "Uploads" THEN o Sistema SHALL exibir área de upload para arquivos CSV ou XLSX
2. WHEN um arquivo é carregado THEN o Sistema SHALL exibir interface de mapeamento entre colunas do arquivo e Campo_Personalizado
3. WHEN o mapeamento é configurado THEN o Sistema SHALL validar que existe coluna obrigatória "id task" mapeada
4. WHEN o usuário confirma o mapeamento THEN o Sistema SHALL adicionar job na Fila_Processamento para atualização
5. WHEN a atualização é processada THEN o Sistema SHALL atualizar Task_ClickUp via API do ClickUp usando os valores mapeados

### Requirement 11

**User Story:** Como um usuário, eu quero acompanhar o progresso das operações através da aba "Relatórios", para que eu possa ver logs e status em tempo real.

#### Acceptance Criteria

1. WHEN um usuário acessa a aba "Relatórios" THEN o Sistema SHALL exibir lista do Historico_Operacao com status atual
2. WHEN uma operação está em andamento THEN o Sistema SHALL exibir barra de progresso atualizada via WebSocket_Conexao
3. WHEN o usuário alterna entre abas THEN o Sistema SHALL manter WebSocket_Conexao ativa e continuar atualizações
4. WHEN o usuário retorna após sair THEN o Sistema SHALL reconectar WebSocket_Conexao e mostrar status atual
5. WHEN uma operação é concluída THEN o Sistema SHALL exibir resumo final com totais de sucesso e erro

### Requirement 12

**User Story:** Como um usuário, eu quero configurar parâmetros do sistema e gerenciar metadados através da aba "Configurações", para que eu possa personalizar o comportamento e manter dados atualizados.

#### Acceptance Criteria

1. WHEN um usuário acessa a aba "Configurações" THEN o Sistema SHALL exibir campo para Token_ClickUp, rate limit, botão "Limpar Histórico" e botão "Atualizar Campos"
2. WHEN o usuário clica em "Atualizar Campos" THEN o Sistema SHALL buscar novamente todos os Metadados_ClickUp e exibir Progresso_Rosca
3. WHEN novos metadados são encontrados THEN o Sistema SHALL adicionar novos Campo_Personalizado e atualizar existentes
4. WHEN o rate limit é configurado THEN o Sistema SHALL permitir valores entre 10-10000 requisições por minuto
5. WHEN o usuário clica em "Limpar Histórico" THEN o Sistema SHALL remover todos os registros do Historico_Operacao após confirmação dupla

### Requirement 13

**User Story:** Como um desenvolvedor, eu quero que o sistema tenha navegação clara entre funcionalidades, para que os usuários possam usar cada aba de forma independente.

#### Acceptance Criteria

1. WHEN o sistema é acessado THEN o Sistema SHALL exibir menu com 4 abas: "Buscar Tasks", "Uploads", "Relatórios", "Configurações"
2. WHEN o usuário navega entre abas THEN o Sistema SHALL manter estado da Sessao_Usuario e WebSocket_Conexao
3. WHEN uma aba é carregada THEN o Sistema SHALL carregar apenas os recursos específicos daquela funcionalidade
4. WHEN ocorre erro em uma aba THEN o Sistema SHALL isolar o erro e não afetar outras abas
5. WHEN operações estão em andamento THEN o Sistema SHALL permitir navegação entre abas sem interromper processamento

### Requirement 14

**User Story:** Como um sistema, eu quero armazenar metadados do ClickUp de forma estruturada, para que a navegação hierárquica e seleção de campos seja eficiente.

#### Acceptance Criteria

1. WHEN Workspace_ClickUp são sincronizados THEN o Sistema SHALL armazenar ID e nome de cada workspace
2. WHEN Space_ClickUp são sincronizados THEN o Sistema SHALL armazenar ID, nome e referência ao workspace pai
3. WHEN Folder_ClickUp são sincronizados THEN o Sistema SHALL armazenar ID, nome e referência ao space pai
4. WHEN Lista_ClickUp são sincronizadas THEN o Sistema SHALL armazenar ID, nome e referência ao folder pai
5. WHEN Campo_Personalizado são sincronizados THEN o Sistema SHALL armazenar ID, nome, tipo, options, orderindex e name das options

### Requirement 15

**User Story:** Como um administrador, eu quero que o sistema tenha performance adequada e limpeza automática, para que não haja acúmulo desnecessário de dados temporários.

#### Acceptance Criteria

1. WHEN jobs são processados THEN o Sistema SHALL limpar dados da Fila_Processamento após conclusão bem-sucedida
2. WHEN jobs falham THEN o Sistema SHALL manter na fila por máximo 24 horas antes da limpeza automática
3. WHEN arquivos temporários são criados THEN o Sistema SHALL remover arquivos após processamento ou em 1 hora
4. WHEN o sistema processa mais de 1000 registros THEN o Sistema SHALL manter uso de memória abaixo de 512MB
5. WHEN múltiplos usuários acessam simultaneamente THEN o Sistema SHALL suportar até 10 conexões WebSocket concorrentes