// SPDX-License-Identifier: Apache-2.0
// solhint-disable func-name-mixedcase
pragma solidity ^0.8.20;

import "forge-std/src/Test.sol";
import {MyERC20} from "../src/MyERC20.sol";
import {Ownable} from "@openzeppelin/contracts/access/Ownable.sol";

contract MyERC20Test is Test {
    MyERC20 private token;

    address private owner;
    address private user;

    function setUp() public {
        owner = makeAddr("owner");
        user = makeAddr("user");

        vm.prank(owner);
        token = new MyERC20();
    }

    function test_mint_Success() public {
        uint256 amount = 100 ether;

        vm.prank(owner);
        token.mint(user, amount);

        assertEq(token.balanceOf(user), amount);
        assertEq(token.totalSupply(), amount);
    }

    function test_mint_RevertWhen_CallerIsNotOwner() public {
        uint256 amount = 100 ether;

        vm.prank(user);

        vm.expectRevert(abi.encodeWithSelector(Ownable.OwnableUnauthorizedAccount.selector, user));
        token.mint(user, amount);
    }
}
