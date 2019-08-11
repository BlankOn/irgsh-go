## Persiapan kunci penanda tangan paket

Keaslian paket di lumbung turunan Debian dibantu oleh verifikasi tanda tangan digital dengan kunci GPG (itu sebabnya alamat lumbung tersebut tidak perlu lagi dilindungi oleh HTTPS/TLS, lihat https://whydoesaptnotusehttps.com/). Kita memerlukan kunci GPG untuk menandatangani paket-paket nantinya. Setelah dibuat sesuai panduan di bawah ini, kunci-kunci ini akan tersimpan di `/.gnugpg`.

### Mempersiapkan `rng` untuk mempercepat generate entropy

```
$ sudo apt-get install rng-tools
$ sudo rngd -r /dev/urandom
```

### Membuat kunci GPG utama.

Abaikan permintaan `passphrase` untuk menunjang otomasi penandatanganan paket.

- `gpg --full-generate-key`
```
gpg (GnuPG) 2.1.18; Copyright (C) 2017 Free Software Foundation, Inc.
This is free software: you are free to change and redistribute it.
There is NO WARRANTY, to the extent permitted by law.

Please select what kind of key you want:
   (1) RSA and RSA (default)
   (2) DSA and Elgamal
   (3) DSA (sign only)
   (4) RSA (sign only)
Your selection? 1
RSA keys may be between 1024 and 4096 bits long.
What keysize do you want? (2048) 4096
Requested keysize is 4096 bits
Please specify how long the key should be valid.
         0 = key does not expire
      <n>  = key expires in n days
      <n>w = key expires in n weeks
      <n>m = key expires in n months
      <n>y = key expires in n years
Key is valid for? (0) 5y
Key expires at Wed Jan 24 04:58:41 2024 EST
Is this correct? (y/N) y

GnuPG needs to construct a user ID to identify your key.

Real name: BlankOn Developer
Email address: blankon-dev@googlegroups.com
Comment:
You selected this USER-ID:
    "BlankOn Developer <blankon-dev@googlegroups.com>"

Change (N)ame, (C)omment, (E)mail or (O)kay/(Q)uit? O
We need to generate a lot of random bytes. It is a good idea to perform
some other action (type on the keyboard, move the mouse, utilize the
disks) during the prime generation; this gives the random number
generator a better chance to gain enough entropy.

gpg: key 17963DC67219B965 marked as ultimately trusted
gpg: revocation certificate stored as '/home/arsipdev-reboot/.gnupg/openpgp-revocs.d/9584C1230204D624A15D215117963DC67219B965.rev'
public and secret key created and signed.
pub   rsa4096 2019-01-25 [SC] [expires: 2024-01-24]
      9584C1230204D624A15D215117963DC67219B965
      9584C1230204D624A15D215117963DC67219B965
uid                      BlankOn Developer <blankon-dev@googlegroups.com>
sub   rsa4096 2019-01-25 [E] [expires: 2024-01-24]
```

### Membuat sub kunci untuk keperluan penandatanganan paket

Parameternya adalah identitas kunci master.

- `gpg --edit-key 05657D94F29BDACB99F6CE7D0B352C08D746A9A6`
```
gpg (GnuPG) 2.1.18; Copyright (C) 2017 Free Software Foundation, Inc.
This is free software: you are free to change and redistribute it.
There is NO WARRANTY, to the extent permitted by law.

Secret key is available.

gpg: checking the trustdb
gpg: marginals needed: 3  completes needed: 1  trust model: pgp
gpg: depth: 0  valid:   2  signed:   0  trust: 0-, 0q, 0n, 0m, 0f, 2u
gpg: next trustdb check due at 2021-01-24
sec  rsa2048/0B352C08D746A9A6
     created: 2019-01-25  expires: 2021-01-24  usage: SC
     trust: ultimate      validity: ultimate
ssb  rsa2048/BE8FF591E6569748
     created: 2019-01-25  expires: 2021-01-24  usage: E
[ultimate] (1). BlankOn Developer <blankon-dev@googlegroups.com>

gpg> addkey
Please select what kind of key you want:
   (3) DSA (sign only)
   (4) RSA (sign only)
   (5) Elgamal (encrypt only)
   (6) RSA (encrypt only)
Your selection? 4
RSA keys may be between 1024 and 4096 bits long.
What keysize do you want? (2048) 4096
Requested keysize is 4096 bits
Please specify how long the key should be valid.
         0 = key does not expire
      <n>  = key expires in n days
      <n>w = key expires in n weeks
      <n>m = key expires in n months
      <n>y = key expires in n years
Key is valid for? (0) 5y
Key expires at Wed Jan 24 05:06:05 2024 EST
Is this correct? (y/N) y
Really create? (y/N) y

We need to generate a lot of random bytes. It is a good idea to perform
some other action (type on the keyboard, move the mouse, utilize the
disks) during the prime generation; this gives the random number
generator a better chance to gain enough entropy.

sec  rsa2048/0B352C08D746A9A6
     created: 2019-01-25  expires: 2021-01-24  usage: SC
     trust: ultimate      validity: ultimate
ssb  rsa2048/BE8FF591E6569748
     created: 2019-01-25  expires: 2021-01-24  usage: E
ssb  rsa4096/1C608FE2ECC8842B
     created: 2019-01-25  expires: 2024-01-24  usage: S
[ultimate] (1). BlankOn Developer <blankon-dev@googlegroups.com>

gpg> save
```

Identitas kunci anak ini (string `0B352C08D746A9A6`) yang akan dipakai di konfigurasi lumbung nantinya.

### Memisahkan kunci master

Tujuan penggunaan subkey dan pemisahan kunci master adalah supaya bila kunci tanda tangan terkena kompromi, kunci penanda tangan baru masih bisa diterbitkan dan paket lama masih bisa diverifikasi.

```
$ gpg --armor --export-secret-key 05657D94F29BDACB99F6CE7D0B352C08D746A9A6 > private.key
$ gpg --armor --export 05657D94F29BDACB99F6CE7D0B352C08D746A9A6 >> private.key
```

Simpan berkas `private.key` ini ke tempat yang aman.

Pisahkan kunci publik master dan kunci privat anak.

```
$ gpg --armor --export 05657D94F29BDACB99F6CE7D0B352C08D746A9A6 > public.key
$ gpg --armor --export-secret-subkeys 0B352C08D746A9A6 > signing.key
```
Hapus kunci privat master dari `gnupg`.

```
$ gpg --delete-secret-key 05657D94F29BDACB99F6CE7D0B352C08D746A9A6
```

Impor kembali kunci publik master dan kunci privat anak.

```
$ gpg --import public.key signing.key
```

Pastikan kunci privat master sudah tidak terdaftar di `gnupg`.

```
$ gpg --list-secret-keys
/home/arsipdev/.gnupg/pubring.kbx
----------------------------------------
sec#  rsa4096 2019-01-25 [SC] [expires: 2024-01-24]
      XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX
uid           [ultimate] BlankOn Developer <blankon-dev@googlegroups.com>
ssb   rsa4096 2019-01-25 [E] [expires: 2024-01-24]
ssb   rsa4096 2019-01-25 [S] [expires: 2024-01-24]
```

Simbol # setelah `sec` menandakan tidak ada kunci privat master di `gnupg`.
