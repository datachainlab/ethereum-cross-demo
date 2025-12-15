// SPDX-License-Identifier: Apache-2.0
// solhint-disable one-contract-per-file, func-name-mixedcase, gas-small-strings, function-max-lines
pragma solidity ^0.8.20;

import "forge-std/src/Test.sol";
import {MyERC20TransferModule} from "../src/MyERC20TransferModule.sol";
import {ERC20TransferModule} from "@datachainlab/cross-solidity/src/example/ERC20TransferModule.sol";
import {CrossContext} from "@datachainlab/cross-solidity/src/core/IContractModule.sol";
import {Account as AuthAccount, AuthType} from "@datachainlab/cross-solidity/src/proto/cross/core/auth/Auth.sol";
import {GoogleProtobufAny} from "@hyperledger-labs/yui-ibc-solidity/contracts/proto/GoogleProtobufAny.sol";

contract MyERC20TransferModuleHarness is MyERC20TransferModule {
    function exposed_authorize(CrossContext calldata context, bytes calldata callInfo) external view {
        _authorize(context, callInfo);
    }
}

contract MyERC20TransferModuleTest is Test {
    MyERC20TransferModuleHarness private harness;

    bytes32 private constant TX_ID_RAW = keccak256("test_tx");
    bytes private txID;
    bytes private callInfo;

    address private user;
    address private receiver;
    uint256 private constant AMOUNT = 100 ether;

    string private constant EXPECTED_TYPE_URL = "/extension.types.SampleAuthExtension";
    string private constant INVALID_TYPE_URL = "/extension.types.InvalidExtension";

    function setUp() public {
        user = makeAddr("user");
        receiver = makeAddr("receiver");
        txID = abi.encode(TX_ID_RAW);

        callInfo = abi.encode(user, receiver, AMOUNT);

        harness = new MyERC20TransferModuleHarness();
        harness.initialize(address(this), address(0x1));
    }

    // --- Helper Functions ---

    function _createCrossContext(
        address[] memory _signers,
        AuthType.AuthMode[] memory _modes,
        string[] memory _typeUrls
    ) internal view returns (CrossContext memory) {
        require(_signers.length == _modes.length && _signers.length == _typeUrls.length, "Array length mismatch");

        AuthAccount.Data[] memory authSigners = new AuthAccount.Data[](_signers.length);

        for (uint256 i = 0; i < _signers.length; ++i) {
            GoogleProtobufAny.Data memory option;

            if (_modes[i] == AuthType.AuthMode.AUTH_MODE_EXTENSION) {
                option = GoogleProtobufAny.Data({type_url: _typeUrls[i], value: ""});
            } else {
                option = GoogleProtobufAny.Data({type_url: "", value: ""});
            }

            authSigners[i] = AuthAccount.Data({
                id: abi.encodePacked(_signers[i]), auth_type: AuthType.Data({mode: _modes[i], option: option})
            });
        }

        return CrossContext({txID: txID, txIndex: 0, signers: authSigners});
    }

    function _createSingleSignerContext(address _signer, AuthType.AuthMode _mode, string memory _typeUrl)
        internal
        view
        returns (CrossContext memory)
    {
        address[] memory signers = new address[](1);
        signers[0] = _signer;

        AuthType.AuthMode[] memory modes = new AuthType.AuthMode[](1);
        modes[0] = _mode;

        string[] memory typeUrls = new string[](1);
        typeUrls[0] = _typeUrl;

        return _createCrossContext(signers, modes, typeUrls);
    }

    // --- Tests ---

    function test_authorize_Success() public {
        CrossContext memory context =
            _createSingleSignerContext(user, AuthType.AuthMode.AUTH_MODE_EXTENSION, EXPECTED_TYPE_URL);

        harness.exposed_authorize(context, callInfo);
    }

    function test_authorize_SuccessWithMultipleSigners() public {
        address otherUser = makeAddr("other");

        address[] memory signers = new address[](2);
        AuthType.AuthMode[] memory modes = new AuthType.AuthMode[](2);
        string[] memory typeUrls = new string[](2);

        signers[0] = otherUser;
        modes[0] = AuthType.AuthMode.AUTH_MODE_EXTENSION;
        typeUrls[0] = EXPECTED_TYPE_URL;

        signers[1] = user;
        modes[1] = AuthType.AuthMode.AUTH_MODE_EXTENSION;
        typeUrls[1] = EXPECTED_TYPE_URL;

        CrossContext memory context = _createCrossContext(signers, modes, typeUrls);

        harness.exposed_authorize(context, callInfo);
    }

    function test_authorize_RevertWhen_SignerAddressMismatch() public {
        address attacker = makeAddr("attacker");

        CrossContext memory context =
            _createSingleSignerContext(attacker, AuthType.AuthMode.AUTH_MODE_EXTENSION, EXPECTED_TYPE_URL);

        vm.expectRevert(ERC20TransferModule.ERC20TransferModuleUnauthorized.selector);
        harness.exposed_authorize(context, callInfo);
    }

    function test_authorize_RevertWhen_AuthModeLocal() public {
        CrossContext memory context = _createSingleSignerContext(user, AuthType.AuthMode.AUTH_MODE_LOCAL, "");

        vm.expectRevert(ERC20TransferModule.ERC20TransferModuleUnauthorized.selector);
        harness.exposed_authorize(context, callInfo);
    }

    function test_authorize_RevertWhen_AuthModeChannel() public {
        CrossContext memory context = _createSingleSignerContext(user, AuthType.AuthMode.AUTH_MODE_CHANNEL, "");

        vm.expectRevert(ERC20TransferModule.ERC20TransferModuleUnauthorized.selector);
        harness.exposed_authorize(context, callInfo);
    }

    function test_authorize_RevertWhen_InvalidTypeUrl() public {
        CrossContext memory context =
            _createSingleSignerContext(user, AuthType.AuthMode.AUTH_MODE_EXTENSION, INVALID_TYPE_URL);

        vm.expectRevert(ERC20TransferModule.ERC20TransferModuleUnauthorized.selector);
        harness.exposed_authorize(context, callInfo);
    }

    function test_authorize_RevertWhen_InvalidSignerIdLength() public {
        CrossContext memory context =
            _createSingleSignerContext(user, AuthType.AuthMode.AUTH_MODE_EXTENSION, EXPECTED_TYPE_URL);
        context.signers[0].id = bytes("invalid_length_id");

        vm.expectRevert(ERC20TransferModule.ERC20TransferModuleUnauthorized.selector);
        harness.exposed_authorize(context, callInfo);
    }

    function test_authorize_RevertWhen_NoSigners() public {
        address[] memory signers = new address[](0);
        AuthType.AuthMode[] memory modes = new AuthType.AuthMode[](0);
        string[] memory typeUrls = new string[](0);

        CrossContext memory context = _createCrossContext(signers, modes, typeUrls);

        vm.expectRevert(ERC20TransferModule.ERC20TransferModuleUnauthorized.selector);
        harness.exposed_authorize(context, callInfo);
    }
}
