#[test_only]
module Std::HashTests {
    use Std::Hash;

    #[test]
    fun sha2_256_expected_hash() {
        let input = x"616263";
        let expected_output = x"ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad";
        assert!(Hash::sha2_256(input) == expected_output, 0);
    }

    #[test]
    fun sha3_256_expected_hash() {
        let input = x"616263";
        let expected_output = x"3a985da74fe225b2045c172d6bd390bd855f086e3e9d525b46bfe24511431532";
        assert!(Hash::sha3_256(input) == expected_output, 0);
    }
}
