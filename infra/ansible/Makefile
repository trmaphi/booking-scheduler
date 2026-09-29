ANSIBLE_CONFIG := $(CURDIR)/ansible.cfg
export ANSIBLE_CONFIG

.PHONY: collections syntax lint test bootstrap bootstrap-root deploy rollback backup restore restore-cutover

collections:
	ansible-galaxy collection install -r requirements.yml

syntax:
	ansible-playbook --syntax-check -i inventory/production.yml playbooks/bootstrap.yml

lint:
	ansible-lint .

test:
	python3 -m unittest discover -s tests -p '*_test.py'

bootstrap:
	@test -n "$(PUBLIC_KEY_FILE)" || (echo "PUBLIC_KEY_FILE is required" >&2; exit 2)
	ansible-playbook -i inventory/production.yml playbooks/bootstrap.yml --extra-vars "bootstrap_public_key=$$(cat '$(PUBLIC_KEY_FILE)')"

# First run only: connect as root with an interactive SSH password prompt.
bootstrap-root:
	@test -n "$(PUBLIC_KEY_FILE)" || (echo "PUBLIC_KEY_FILE is required" >&2; exit 2)
	ansible-playbook -i inventory/production.yml playbooks/bootstrap.yml --user root --ask-pass --extra-vars "bootstrap_public_key=$$(cat '$(PUBLIC_KEY_FILE)')"

deploy:
	@test -n "$(RELEASE_TAG)" || (echo "RELEASE_TAG is required" >&2; exit 2)
	ansible-playbook -i inventory/production.yml playbooks/deploy.yml --ask-vault-pass --extra-vars @group_vars/production.yml --extra-vars @group_vars/production.vault.yml --extra-vars "release_tag=$(RELEASE_TAG)"

rollback:
	@test -n "$(ROLLBACK_TAG)" || (echo "ROLLBACK_TAG is required" >&2; exit 2)
	ansible-playbook -i inventory/production.yml playbooks/rollback.yml --ask-vault-pass --extra-vars @group_vars/production.yml --extra-vars @group_vars/production.vault.yml --extra-vars "rollback_tag=$(ROLLBACK_TAG)"

backup:
	ansible-playbook -i inventory/production.yml playbooks/backup.yml --ask-vault-pass --extra-vars @group_vars/production.yml --extra-vars @group_vars/production.vault.yml

restore:
	@test -n "$(SNAPSHOT_ID)" || (echo "SNAPSHOT_ID is required" >&2; exit 2)
	ansible-playbook -i inventory/production.yml playbooks/restore.yml --ask-vault-pass --extra-vars @group_vars/production.yml --extra-vars @group_vars/production.vault.yml --extra-vars "restore_snapshot_id=$(SNAPSHOT_ID) confirm_restore_cutover=true"

restore-cutover:
	@test -n "$(SNAPSHOT_ID)" || (echo "SNAPSHOT_ID is required" >&2; exit 2)
	ansible-playbook -i inventory/production.yml playbooks/restore.yml --tags restore_cutover --ask-vault-pass --extra-vars @group_vars/production.yml --extra-vars @group_vars/production.vault.yml --extra-vars "restore_snapshot_id=$(SNAPSHOT_ID) confirm_restore_cutover=true"
