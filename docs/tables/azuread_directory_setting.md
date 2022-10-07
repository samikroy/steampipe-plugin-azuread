# Table: azuread_directory_setting

Directory settings define the configurations that can be used to customize the tenant-wide and object-specific restrictions and allowed behavior.

By default, all entities inherit the preset defaults.

## Examples

### Basic info

```sql
select
  display_name,
  id,
  values
from
  azuread_directory_setting;
```

### Check user admin consent workflow is enabled

```sql
select
  display_name,
  id,
  values
from
  azuread_directory_setting
where
  display_name = 'Consent Policy Settings'
  and values @> '[{"name": "EnableAdminConsentRequests","value": "true"}]';
```

### Check password protection is enabled for active directory

```sql
select
  display_name,
  id,
  values
from
  azuread_directory_setting
where
  display_name = 'Password Rule Settings'
  and values @> '[{"name": "EnableBannedPasswordCheckOnPremises","value": "True"}]'
  and values @> '[{"name": "BannedPasswordCheckOnPremisesMode","value": "Enforced"}]';
```