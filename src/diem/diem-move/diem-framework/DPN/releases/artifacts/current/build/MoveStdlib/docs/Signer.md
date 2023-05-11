
<a name="0x1_Signer"></a>

# Module `0x1::Signer`



-  [Function `borrow_address`](#0x1_Signer_borrow_address)
-  [Function `address_of`](#0x1_Signer_address_of)
-  [Module Specification](#@Module_Specification_0)


<pre><code></code></pre>



<a name="0x1_Signer_borrow_address"></a>

## Function `borrow_address`



<pre><code><b>public</b> <b>fun</b> <a href="Signer.md#0x1_Signer_borrow_address">borrow_address</a>(s: &signer): &address
</code></pre>



<details>
<summary>Implementation</summary>


<pre><code><b>native</b> <b>public</b> <b>fun</b> <a href="Signer.md#0x1_Signer_borrow_address">borrow_address</a>(s: &signer): &address;
</code></pre>



</details>

<a name="0x1_Signer_address_of"></a>

## Function `address_of`



<pre><code><b>public</b> <b>fun</b> <a href="Signer.md#0x1_Signer_address_of">address_of</a>(s: &signer): address
</code></pre>



<details>
<summary>Implementation</summary>


<pre><code><b>public</b> <b>fun</b> <a href="Signer.md#0x1_Signer_address_of">address_of</a>(s: &signer): address {
    *<a href="Signer.md#0x1_Signer_borrow_address">borrow_address</a>(s)
}
</code></pre>



</details>

<a name="@Module_Specification_0"></a>

## Module Specification

Return true only if <code>s</code> is a transaction signer. This is a spec function only available in spec.


<a name="0x1_Signer_is_txn_signer"></a>


<pre><code><b>native</b> <b>fun</b> <a href="Signer.md#0x1_Signer_is_txn_signer">is_txn_signer</a>(s: signer): bool;
</code></pre>


Return true only if <code>a</code> is a transaction signer address. This is a spec function only available in spec.


<a name="0x1_Signer_is_txn_signer_addr"></a>


<pre><code><b>native</b> <b>fun</b> <a href="Signer.md#0x1_Signer_is_txn_signer_addr">is_txn_signer_addr</a>(a: address): bool;
</code></pre>


[//]: # ("File containing references which can be used from documentation")
